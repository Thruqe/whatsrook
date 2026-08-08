# AGENTS Architecture & Guidelines

Welcome to WhatsRook.

## Core Architecture

- **CLI Entrypoint**: Located in [`cli/main.go`](file:///home/thruqe/whatsrook/cli/main.go), which parses command-line arguments and initializes `whatsrook.NewRookClient(config)`.
- **Package Layout**:
  - `whatsrook`: Root library package containing `RookClient`, bot lifecycle, WebSocket hub, IPC stanzas, and event routing.
  - `whatsrook/plugins`: Command registration (`Register`), dispatching, and structured error handling (`plugins/error.go`).
  - `whatsrook/send`: High-level sending abstractions and context management (`PluginContext`).
  - `whatsrook/wa-core`: WhatsApp protocol core library and database stores (`wa-core/store/sqlstore`).
  - `whatsrook/caller`: Local VoIP call signaling and WebRTC media engine.
  - `whatsrook/utils`: Media processing, network guards, font formatting, and waveform generators.

## Plugin Authoring

Plugins take advantage of `plugins/error.go` for error handling:
```go
func handleCommand(ctx *plugins.Context) error {
    if !ctx.HasArgs() {
        return plugins.ErrUsage("mycommand <query>")
    }
    return ctx.Reply("Success")
}
```

## Relevant Documentation

- [Docs](./Documentation/README.md)
- [Security](./SECURITY.md)
- [Patch System](./patch/README.md)
