package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Thruqe/whatsrook/store/sqlstore"
	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func main() {
	cli := parseArgs()

	logLevel := slog.LevelInfo
	if cli.Debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	if cli.Dev {
		slog.Warn("dev mode enabled — WebSocket CORS origin check disabled")
	}

	if err := os.MkdirAll(cli.AuthDir, 0755); err != nil {
		slog.Error("failed to create auth dir", "err", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(cli.AuthDir, cli.Session+".db")

	if cli.Logout {
		fmt.Printf("Logging out session: %s\n", cli.Session)
		for _, suffix := range []string{"", "-shm", "-wal"} {
			path := dbPath + suffix
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", path, err)
			}
		}
		fmt.Println("Session cleared.")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown — just cancel; let runSession and defer close the db cleanly
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	// DB + whatsmeow client
	waLevel := "INFO"
	if cli.Debug {
		waLevel = "DEBUG"
	}
	dbLog := waLog.Stdout("Database", waLevel, true)
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), dbLog)
	if err != nil {
		slog.Error("failed to open db", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := container.Close(); err != nil {
			slog.Error("failed to close db", "err", err)
		}
	}()

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		slog.Error("failed to get device", "err", err)
		os.Exit(1)
	}

	clientLog := waLog.Stdout("Client", waLevel, true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// WebSocket hub + HTTP server
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

	bot := newBot(client, hub, cli)
	if err := bot.run(ctx); err != nil {
		slog.Error("session error", "err", err)
		os.Exit(1)
	}
}
