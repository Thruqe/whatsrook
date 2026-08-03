// Bot lifecycle, database initialization, HTTP/WebSocket server, and session management.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"whatsrook/logger"
	commands "whatsrook/plugins"
	"whatsrook/store/sqlstore"
	"whatsrook/updater"
	"whatsrook/utils"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	_ "modernc.org/sqlite"
)

// main executes the WhatsRook lifecycle.
func main() {
	cli := parseArgs()

	if cli.Update {
		fmt.Println("Checking for application update...")
		res, err := updater.PerformUpdate(false)
		if err != nil {
			slog.Error("update failed", "err", err)
			os.Exit(1)
		}

		fmt.Println(res.Message)
		if res.Updated {
			fmt.Println("Restarting process...")
			if err := updater.RestartProcess(); err != nil {
				slog.Error("failed to restart process", "err", err)
				os.Exit(1)
			}
		}
		return
	}

	sessionDir := filepath.Join("auth", cli.Session)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		slog.Error("failed to create session dir", "path", sessionDir, "err", err)
		os.Exit(1)
	}

	if err := logger.InitLogger(sessionDir, cli.Verbose); err != nil {
		slog.Error("failed to initialize logger", "err", err)
		os.Exit(1)
	}
	defer logger.Close()

	dbPath := filepath.Join(sessionDir, cli.Session+".db")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background network health guard & auto-pause manager.
	utils.StartNetworkGuard(ctx, 10*time.Second)

	// Graceful shutdown on Ctrl+C / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutting down process")
		cancel()
	}()

	waLevel := "INFO"
	if cli.Verbose {
		waLevel = "DEBUG"
	}

	// WebSocket hub + HTTP server (shared across retries).
	hub := newHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS(false))

	startPort := 3000
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
		slog.Error("failed to find an available port to bind HTTP server")
		os.Exit(1)
	}

	if actualPort != startPort {
		slog.Warn("port in use — switched to alternative port", "original_port", startPort, "new_port", actualPort)
	}

	server := &http.Server{
		Handler: mux,
	}
	go func() {
		slog.Info("listening", "port", actualPort, "session", cli.Session)
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
		}
	}()

	for {
		err := runSession(ctx, cli, sessionDir, dbPath, waLevel, hub)

		// Clean shutdown or context cancelled — exit normally.
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}

		// Pairing stalled (malformed WA notification). Wipe the session and retry.
		if errors.Is(err, ErrPairTimeout) {
			slog.Error("session error", "err", "Pairing timed out — WhatsApp sent a bad response.")
			slog.Warn("session action", "warn", "The session directory will be cleared and a new code generated.")

			wipeSession(sessionDir)

			for i := 10; i > 0; i-- {
				fmt.Printf("\r  Retrying in %2ds…", i)
				select {
				case <-time.After(time.Second):
				case <-ctx.Done():
					fmt.Println()
					return
				}
			}
			fmt.Println("\r  Retrying now…         ")
			continue
		}

		// Any other error is fatal.
		slog.Error("session error", "err", err)
		os.Exit(1)
	}
}

// runSession opens the DB, creates a whatsmeow client, handles --logout, then
// runs the bot. It returns ErrPairTimeout when --pair stalls so the caller
// can wipe + retry, or nil on clean shutdown.
func runSession(ctx context.Context, cli Arguments, sessionDir, dbPath, waLevel string, hub *Hub) error {
	dbLog := logger.WhatsmeowStyle("Database", waLevel, true)
	container, err := sqlstore.New(ctx, "sqlite", fmt.Sprintf(
		"file:%s?_pragma=busy_timeout=5000&_pragma=journal_mode=WAL&_pragma=synchronous=NORMAL&_pragma=foreign_keys=on&_pragma=cache_size=-2000",
		dbPath,
	), dbLog)
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

	clientLog := logger.WhatsmeowStyle("Client", waLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// Initialize meowcaller before connecting whatsmeow so raw call adapter hook is installed
	commands.RegisterMeowCaller(client)

	// ── Logout
	if cli.Logout {
		slog.Info("logging out session", "session", cli.Session)

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
		wipeSession(sessionDir)
		slog.Info("session directory cleared successfully", "session", cli.Session)
		return nil
	}

	// ── Normal / pair run
	bot := newBot(client, hub, cli)
	return bot.run(ctx)
}

// wipeSession removes the session folder and all contained database/cache files.
func wipeSession(sessionDir string) {
	if err := os.RemoveAll(sessionDir); err != nil && !os.IsNotExist(err) {
		slog.Error("failed to remove session directory", "path", sessionDir, "err", err)
		return
	}
	// Recreate empty folder so future writes in the same run don't fail
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		slog.Error("failed to recreate session directory", "path", sessionDir, "err", err)
	}
}
