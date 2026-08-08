// Bot lifecycle, database initialization, HTTP/WebSocket server, and session management.
package whatsrook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	commands "whatsrook/cli/plugins"
	"whatsrook/utils"
	"whatsrook/wa-core/store/sqlstore"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
	"whatsrook/wa-core"
	"whatsrook/wa-core/types/events"
)

// ClientType represents the platform emulated by the WhatsApp client.
type ClientType int

const (
	ClientChrome ClientType = iota
	ClientAndroid
	ClientIos
)

// ParseClientType converts a platform name string to its ClientType enum.
func ParseClientType(s string) (ClientType, bool) {
	c, ok := map[string]ClientType{
		"chrome":  ClientChrome,
		"android": ClientAndroid,
		"ios":     ClientIos,
	}[strings.ToLower(s)]
	return c, ok
}

// Config holds configuration parameters for a RookClient instance.
type Config struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	Verbose         bool
	ClientType      ClientType
	SkipOldMessages bool
	DataDir         string // Directory to store session data (default: "auth")
	WSPort          int    // Base port for HTTP/WebSocket server (default: 3000)
}

// RookClient manages the WhatsRook bot lifecycle, event routing, database connection, and WebSocket server.
type RookClient struct {
	Config Config

	hub        *Hub
	client     *whatsmeow.Client
	bot        *Bot
	httpServer *http.Server
	listener   net.Listener
	mu         sync.Mutex
}

// NewRookClient returns a new RookClient initialized with the provided Config and sensible defaults.
func NewRookClient(cfg Config) *RookClient {
	r := &RookClient{Config: cfg}
	r.applyDefaults()
	return r
}

func (r *RookClient) applyDefaults() {
	if r.Config.DataDir == "" {
		r.Config.DataDir = "auth"
	}
	if r.Config.WSPort <= 0 {
		r.Config.WSPort = 3000
	}
}

// Client returns the underlying whatsmeow Client instance (available after connection/start).
func (r *RookClient) Client() *whatsmeow.Client {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.client
}

// Hub returns the central WebSocket event and control hub.
func (r *RookClient) Hub() *Hub {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hub
}

// Start launches the full RookClient lifecycle (DB connection, HTTP/WS server, network guard, and WhatsApp engine).
func (r *RookClient) Start(ctx context.Context) error {
	r.applyDefaults()

	if r.Config.Session == "" {
		return errors.New("session phone number is required")
	}

	sessionDir := filepath.Join(r.Config.DataDir, r.Config.Session)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return fmt.Errorf("failed to create session dir %q: %w", sessionDir, err)
	}

	if err := utils.InitLogger(sessionDir, r.Config.Verbose); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer utils.CloseLogger()

	dbPath := filepath.Join(sessionDir, r.Config.Session+".db")

	// Start background network health guard & auto-pause manager.
	utils.StartNetworkGuard(ctx, 10*time.Second)

	waLevel := "INFO"
	if r.Config.Verbose {
		waLevel = "DEBUG"
	}

	// WebSocket hub + HTTP server (shared across retries).
	hub := newHub()
	r.mu.Lock()
	r.hub = hub
	r.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS(false))

	startPort := r.Config.WSPort
	var listener net.Listener
	var actualPort int
	for p := startPort; p < startPort+100; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener = l
			actualPort = p
			break
		}
		if p == startPort {
			slog.Warn("port in use, attempting to bind alternative port", "attempted_port", p, "err", err)
		}
	}

	if listener == nil {
		return errors.New("failed to find an available port to bind HTTP server")
	}

	if actualPort != startPort {
		slog.Warn("port in use — switched to alternative port", "original_port", startPort, "new_port", actualPort)
	}

	r.listener = listener
	server := &http.Server{Handler: mux}
	r.httpServer = server

	go func() {
		slog.Info("listening", "port", actualPort, "session", r.Config.Session)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	for {
		err := r.runSession(ctx, sessionDir, dbPath, waLevel)

		// Clean shutdown or context cancelled — exit normally.
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		// Pairing stalled (malformed WA notification). Wipe the session and retry.
		if errors.Is(err, ErrPairTimeout) {
			slog.Error("session error", "err", "Pairing timed out — WhatsApp sent a bad response.")
			slog.Warn("session action", "warn", "The session directory will be cleared and a new code generated.")

			WipeSession(sessionDir)

			for i := 10; i > 0; i-- {
				fmt.Printf("\r  Retrying in %2ds…", i)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					fmt.Println()
					return nil
				}
			}
			fmt.Println("\r  Retrying now…         ")
			continue
		}

		// Any other error is fatal.
		return fmt.Errorf("session error: %w", err)
	}
}

// runSession opens the DB, creates a whatsmeow client, handles logout if configured, then runs the bot.
func (r *RookClient) runSession(ctx context.Context, sessionDir, dbPath, waLevel string) error {
	container, err := r.initStore(ctx, dbPath, waLevel)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	defer func() {
		if err := container.Close(); err != nil {
			slog.Error("failed to close db", "err", err)
		}
	}()

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}
	if sqlStore, ok := deviceStore.Identities.(*sqlstore.SQLStore); ok {
		sqlStore.SessionDir = sessionDir
	}

	clientLog := utils.WhatsmeowStyle("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	r.mu.Lock()
	r.client = client
	r.mu.Unlock()

	// Initialize meowcaller before connecting whatsmeow so raw call adapter hook is installed
	commands.RegisterMeowCaller(client)

	// ── Logout
	if r.Config.Logout {
		slog.Info("logging out session", "session", r.Config.Session)

		if deviceStore.ID == nil {
			slog.Info("session was never paired, skipping server logout")
		} else {
			connected := make(chan struct{}, 1)
			client.AddEventHandler(func(evt any) {
				if _, ok := evt.(*events.Connected); ok {
					select {
					case connected <- struct{}{}:
					default:
					}
				}
			})

			if err := client.Connect(); err != nil {
				slog.Warn("connect failed before logout, wiping local files only", "err", err)
			} else {
				logoutCtx, logoutCancel := context.WithTimeout(ctx, 10*time.Second)
				select {
				case <-connected:
					slog.Info("connected — sending logout to WhatsApp servers")
				case <-logoutCtx.Done():
					slog.Warn("timed out waiting for connection, sending logout anyway")
				}
				logoutCancel()

				if err := client.Logout(ctx); err != nil {
					slog.Warn("server logout returned error", "err", err)
				}
				client.Disconnect()
			}
		}

		// Close DB explicitly before file deletion
		_ = container.Close()
		WipeSession(sessionDir)
		slog.Info("session directory cleared successfully", "session", r.Config.Session)
		return nil
	}

	// ── Normal / pair run
	bot := newBot(client, r.hub, r.Config)
	r.mu.Lock()
	r.bot = bot
	r.mu.Unlock()

	return bot.run(ctx)
}

// WipeSession removes the session folder and all contained database/cache files.
func WipeSession(sessionDir string) {
	if err := os.RemoveAll(sessionDir); err != nil && !os.IsNotExist(err) {
		slog.Error("failed to remove session directory", "path", sessionDir, "err", err)
		return
	}
	// Recreate empty folder so future writes in the same run don't fail
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		slog.Error("failed to recreate session directory", "path", sessionDir, "err", err)
	}
}

func sanitizeDBURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if strings.HasPrefix(rawURL, "postgres://") || strings.HasPrefix(rawURL, "postgresql://") {
		parts := strings.SplitN(rawURL, "@", 2)
		if len(parts) == 2 {
			schemeUser := parts[0]
			subParts := strings.SplitN(schemeUser, ":", 3)
			if len(subParts) == 3 {
				return fmt.Sprintf("%s:%s:****@%s", subParts[0], subParts[1], parts[1])
			}
			return fmt.Sprintf("%s:****@%s", parts[0], parts[1])
		}
	}
	return rawURL
}

func (r *RookClient) initStore(ctx context.Context, dbPath, waLevel string) (*sqlstore.Container, error) {
	dbLog := utils.WhatsmeowStyle("Database", waLevel, true)

	pgURL := os.Getenv("DATABASE_URL")
	if pgURL == "" {
		pgURL = os.Getenv("POSTGRES_URL")
	}
	if pgURL == "" {
		pgURL = os.Getenv("DB_URL")
	}

	// 1. Try PostgreSQL first if explicit DATABASE_URL is set (and not "sqlite" or "none")
	if pgURL != "" && pgURL != "sqlite" && pgURL != "none" {
		slog.Info("attempting connection to PostgreSQL database...", "url", sanitizeDBURL(pgURL))
		container, err := sqlstore.New(ctx, "postgres", pgURL, dbLog)
		if err == nil && container != nil {
			slog.Info("successfully connected to PostgreSQL database")
			return container, nil
		}
		slog.Warn("PostgreSQL connection failed, falling back to SQLite", "err", err)
	}

	// Try default local postgres URL if no explicit DATABASE_URL was provided
	if pgURL == "" {
		defaultPG := "postgres://postgres:postgres@localhost:5432/whatsrook?sslmode=disable"
		slog.Info("attempting connection to default PostgreSQL database...", "url", sanitizeDBURL(defaultPG))
		container, err := sqlstore.New(ctx, "postgres", defaultPG, dbLog)
		if err == nil && container != nil {
			slog.Info("successfully connected to default PostgreSQL database")
			return container, nil
		}
		slog.Warn("default PostgreSQL unavailable, falling back to SQLite", "err", err)
	}

	// 2. Fallback to SQLite
	slog.Info("initializing SQLite database store", "path", dbPath)
	sqliteURI := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout=5000&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL&_pragma=foreign_keys=on&_pragma=cache_size=-2000",
		dbPath,
	)
	container, err := sqlstore.New(ctx, "sqlite", sqliteURI, dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	return container, nil
}
