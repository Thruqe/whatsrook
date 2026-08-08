# AGENTS Architecture & Guidelines

Welcome to WhatsRook.

## Core Architecture

- **CLI Entrypoint**: Located in [`cli/main.go`](cli/main.go), which parses command-line arguments and initializes `whatsrook.NewRookClient(config)`.
- **Package Layout**:
  - `whatsrook`: Root library package containing `RookClient`, bot lifecycle, WebSocket hub, IPC stanzas, and event routing.
  - `whatsrook/cli/plugins`: Command registration (`Register`), dispatching, and structured error handling (`cli/plugins/error.go`).
  - `whatsrook/cli/updater`: Application auto-updater and atomic rollback engine.
  - `whatsrook/cli/resources`: Static CLI resources, audio, and tutorial media assets.
  - `whatsrook/messaging`: High-level sending abstractions, message dispatch, and context management (`PluginContext`).
  - `whatsrook/wa-core`: WhatsApp protocol core library and database stores (`wa-core/store/sqlstore`).
  - `whatsrook/caller`: Local VoIP call signaling and WebRTC media engine.
  - `whatsrook/utils`: Media processing, network guards, font formatting, logging (`utils/logger.go`), and waveform generators.

## Plugin Authoring

Plugins take advantage of `cli/plugins/error.go` for error handling:
```go
func handleCommand(ctx *plugins.Context) error {
    if !ctx.HasArgs() {
        return plugins.ErrUsage("mycommand <query>")
    }
    return ctx.Reply("Success")
}
```

## Relevant Documentation

- [Docs](https://github.com/Thruqe/whatsrook-docs)
- [Security](./SECURITY.md)
- [Patch System](./patch/README.md)
