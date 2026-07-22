package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Thruqe/whatsrook/logger"
	"github.com/Thruqe/whatsrook/store/sqlstore"
	"github.com/Thruqe/whatsrook/updater"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	cli := parseArgs()

	if cli.Update {
		fmt.Println("Checking for application update...")
		res, err := updater.PerformUpdate(false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(res.Message)
		if res.Updated {
			fmt.Println("Restarting process...")
			if err := updater.RestartProcess(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to restart process: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	if err := logger.InitLogger(cli.Debug || cli.Verbose); err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()

	if cli.Dev {
		slog.Warn("dev mode enabled — WebSocket CORS origin check disabled")
	}

	if err := os.MkdirAll(cli.AuthDir, 0755); err != nil {
		slog.Error("failed to create auth dir", "err", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(cli.AuthDir, cli.Session+".db")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown on Ctrl+C / SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	waLevel := "INFO"
	if cli.Debug {
		waLevel = "DEBUG"
	}

	// WebSocket hub + HTTP server (shared across retries).
	hub := newHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS(cli.Dev))
	server := &http.Server{
		Addr:    "0.0.0.0:" + cli.Port,
		Handler: mux,
	}
	go func() {
		slog.Info("listening", "port", cli.Port, "session", cli.Session)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "err", err)
		}
	}()

	for {
		err := runSession(ctx, cli, dbPath, waLevel, hub)

		// Clean shutdown or context cancelled — exit normally.
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}

		// Pairing stalled (malformed WA notification). Wipe the session and retry.
		if errors.Is(err, ErrPairTimeout) {
			fmt.Println()
			fmt.Println("┌─────────────────────────────────────────────────────────┐")
			fmt.Println("│    Pairing timed out — WhatsApp sent a bad response.   │")
			fmt.Println("│  The session will be cleared and a new code generated.  │")
			fmt.Println("└─────────────────────────────────────────────────────────┘")

			wipeSessionFiles(dbPath)

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
// runs the bot.  It returns ErrPairTimeout when --pair stalls so the caller
// can wipe + retry, or nil on clean shutdown.
func runSession(ctx context.Context, cli CliArgs, dbPath, waLevel string, hub *Hub) error {
	dbLog := waLog.Stdout("Database", waLevel, true)
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_pragma=cache_size(-2000)", dbPath), dbLog)
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

	clientLog := waLog.Stdout("Client", waLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// ── Logout flow
	if cli.Logout {
		fmt.Printf("Logging out session: %s\n", cli.Session)

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

		// Close DB explicitly before file deletion (defer would also do it, but
		// we want the files truly released before os.Remove).
		_ = container.Close()
		wipeSessionFiles(dbPath)
		fmt.Println("Session cleared.")
		return nil
	}

	// ── Normal / pair run
	bot := newBot(client, hub, cli)
	return bot.run(ctx)
}

// wipeSessionFiles removes the SQLite database and its WAL/SHM sidecar files.
func wipeSessionFiles(dbPath string) {
	for _, suffix := range []string{"", "-shm", "-wal"} {
		path := dbPath + suffix
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", path, err)
		}
	}
}
