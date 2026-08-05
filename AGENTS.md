# AGENTS Architecture & Guidelines

Welcome to WhatsRook.

## Core Architecture

- **CLI Entrypoint**: Located in [`cli/main.go`](file:///home/thruqe/Documents/whatsrook/cli/main.go), which delegates execution to `whatsrook.ExecuteCLI()`.
- **Package Layout**:
  - `whatsrook`: Root package containing core bot lifecycle, WebSocket hub, IPC stanzas, and event routing.
  - `whatsrook/plugins`: Command registration (`Register`), dispatching, and structured error handling (`plugins/error.go`).
  - `whatsrook/send`: High-level sending abstractions and context management (`PluginContext`).
  - `whatsrook/store/sqlstore`: Custom prefix-free SQLite/PostgreSQL data store and migration upgrades.
  - `whatsrook/patch`: Overrides and patch application scripts for `whatsmeow` defaults.
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
