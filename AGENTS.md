# AGENTS

Welcome to WhatsRook! This document is a guide for AI coding assistants (Claude Code, Cursor, Aider, etc.) to understand our design philosophy, codebase structure, and how to write clean, contribution-ready code.

## What is WhatsRook?

WhatsRook is a lightweight, long-running Go daemon that interfaces with WhatsApp using [whatsmeow](https://github.com/tulir/whatsmeow). It behaves both as a standalone WhatsApp bot and a bridge for external applications:
1. **In-Bot Commands (`./commands`)**: Handles events directly inside the bot (e.g., prefix commands, sticker reactions, group moderation, status/view-once auto-saving).
2. **WebSocket Gateway (`/ws`)**: Streams real-time WhatsApp events out to other applications and accepts control commands (e.g., sending/editing/revoking messages, reactions).

## Our Programming Style & Philosophy

We value simplicity, pragmatism, and raw speed. If you contribute code, please align with these design principles:
* **Pragmatic Go**: Use the Go standard library where possible. We prefer simple, direct code over complex abstractions, interfaces-for-everything, or bloated dependencies.
* **Keep Database Access Simple**: We use SQLite via `sqlstore`. We don't use heavy ORMs; instead, write clean, raw SQL queries using `db.Exec` or `db.QueryRow` to keep operations fast and visible.
* **Concurrency & Memory Safety**: WhatsRook runs continuously. Always avoid leaking goroutines or letting database connections hang open. Clean up temporary files, close readers/writers, and ensure shared state is access-safe (e.g., using mutexes or `sync.Once`).
* **Direct Communication**: Use `ctx.Reply("...")` to communicate back to users in command handlers. Keep error messages clear and user-friendly.
* **Command Creation Style**: Do not use emojis or custom formatting like * when writing in strings for the commands, keep it plain and simple.

## Codebase Map

* [main.go](./main.go): Application entrypoint launcher. Automatically builds `whatsrook` if missing and delegates execution to `entrypoint.sh` or `client.go`.
* [client.go](./client.go): Contains the core WhatsRook daemon lifecycle, database initialization, HTTP/WebSocket server, and session management.
* [cli.go](./cli.go): Manages command line flags (`--session`, `--pair`, `--port`, `--auth-dir`, `--client`, `--qrcode`, `--logout`, `--debug`, `--verbose`, `--dev`).
* [session.go](./session.go): Controls the lifecycle of the WhatsApp connection, including QR/pairing-code registration, event handling, and executing WebSocket control commands.
* [ws.go](./ws.go): Implements the WebSocket connection `Hub` for managing real-time connections, concurrent broadcasting, and safe read/write loops.
* [messages.go](./messages.go): Schema mapping for JSON-based WebSocket payloads.
* [proto/](./proto/):
  * [ws.proto](./proto/ws.proto): Protocol Buffer definitions for WebSocket control frames, event frames, and typed payload messages.
* [example/](./example/):
  * [client.go](./example/client.go): Working demonstration of connecting to WhatsRook, decoding JSON & Protobuf WebSocket messages, and sending control commands.
* [commands/](./commands/):
  * [commands.go](./commands/commands.go): Registers command handlers via an `init()` block using `Register(&Command{...})`.
  * [dispatch.go](./commands/dispatch.go): The entry point for incoming events. It parses messages, matches prefixes, runs moderation triggers, and routes valid commands asynchronously.
  * [helper.go](./commands/helper.go): Internal command helpers (e.g. sending raw responses or retrieving configuration settings).
* [utils/](./utils/):
  * [utils.go](./utils/utils.go): Shared helper functions (FFmpeg audio transcoding, ffprobe audio duration, URL matching, JID sanitization, message extraction).

## Agent Guidelines & Validation

Before you declare your work complete, make sure you perform these verification checks:
1. **Format Code**: Run `go fmt ./...`.
2. **Lint & Vet**:
   * Run `go vet ./...` to check for common issues.
   * Run `golangci-lint run` (or your local equivalent tool) to ensure code quality rules are followed.
3. **Run Tests**: Verify everything works by running `go test ./...`.
4. **Build the Binary**: Make sure the project compiles fine using `go build -o whatsrook .`.