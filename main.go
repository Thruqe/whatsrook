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

	"github.com/Thruqe/zevBot/store/sqlstore"
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

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
		// clean up WAL/SHM
		for _, suffix := range []string{"-shm", "-wal"} {
			path := dbPath + suffix
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Failed to remove %s: %v\n", path, err)
			}
		}
		os.Exit(0)
	}()

	// DB + whatsmeow client
	dbLog := waLog.Stdout("Database", "INFO", true)
	if cli.Debug {
		dbLog = waLog.Stdout("Database", "DEBUG", true)
	}
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath), dbLog)
	if err != nil {
		slog.Error("failed to open db", "err", err)
		os.Exit(1)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		slog.Error("failed to get device", "err", err)
		os.Exit(1)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	if cli.Debug {
		clientLog = waLog.Stdout("Client", "DEBUG", true)
	}
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// WebSocket hub + HTTP server
	hub := newHub()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", hub.ServeWS)

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

	if err := runSession(ctx, client, cli, hub); err != nil {
		slog.Error("session error", "err", err)
		os.Exit(1)
	}
}
