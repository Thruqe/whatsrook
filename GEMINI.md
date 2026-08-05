# GEMINI.md

Welcome to the WhatsRook project.

## Startup Instructions

At the beginning of every session, you MUST read and follow these files to understand the project structure, security policy, and contributing guidelines:

- [AGENTS.md](./AGENTS.md) - Deep codebase architecture, layout, and how to write new commands.
- [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) - Expected standard of behavior.
- [SECURITY.md](./SECURITY.md) - Crucial security policies regarding credentials, memory safety, and acceptable use.

## Command Reference & Makefile

Use the provided `Makefile` or standard Go commands:

- `make start ARGS="-s <session_phone>"` - Run the application via `./cli`
- `make build` - Build the executable binary into `bin/whatsrook`
- `make test` - Run full unit and integration test suite (`go test -v ./...`)
- `make format` - Format code (`go fmt ./...`)
- `make vet` - Run static code analysis (`go vet ./...`)
- `make update` - Upgrade all Go dependencies (`go get -u ./... && go mod tidy`)
- `make patch` - Apply WhatsRook custom patches to `whatsmeow`/`sqlstore`
