# WhatsRook

> [!CAUTION]
> Educational project only. See [DISCLAIMER.md](DISCLAIMER.md) before use.

Read [Documentation](./Documentation/README.md) for detailed architecture, CLI usage, WebSocket IPC specification, plugin development, and deployment guides.

Real-time WhatsApp API built on [whatsmeow](https://github.com/tulir/whatsmeow).

[![Go Code Quality & Tests](https://github.com/Thruqe/whatsrook/actions/workflows/go-checks.yml/badge.svg)](https://github.com/Thruqe/whatsrook/actions/workflows/go-checks.yml)
[![Go Version](https://badgen.net/badge/Go/1.26.4/blue)](https://github.com/Thruqe/whatsrook/blob/master/go.mod)
[![Release](https://badgen.net/badge/Release/v5.0.0/orange)](https://github.com/Thruqe/whatsrook/releases)
[![License](https://badgen.net/badge/License/MIT/yellow)](LICENSE)
[![Join WhatsApp Channel](https://img.shields.io/badge/WhatsApp-Support%20Channel-25D366?style=flat&logo=whatsapp&logoColor=white)](https://whatsapp.com/channel/0029Vb8Vo0k0bIdsTOTF1G2o)

## Features

- **Abstract Plugin Engine**: Easy plugin development taking full advantage of [`plugins/error.go`](file:///home/thruqe/Documents/whatsrook/plugins/error.go) and [`whatsrook/send`](file:///home/thruqe/Documents/whatsrook/send) abstractions.
- **Unified Makefile**: Simple commands (`make start`, `make build`, `make test`, `make format`, `make vet`, `make update`, `make patch`).
- **CLI Architecture**: Clean CLI launcher isolated under [`cli/main.go`](file:///home/thruqe/Documents/whatsrook/cli/main.go).
- **WhatsMeow Patching**: Custom `patch/` system maintaining prefix-free database schemas and custom overrides.
- **Voice Note Engine**: Automatic 64-bin frequency-normalized waveform generation for Opus PTT voice notes.

## Quick Start (Makefile)

```bash
# Install dependencies
make install

# Run application
make start ARGS="-s 1234567890"

# Build binary
make build

# Run test suite
make test
```

## Deployment

### 1. Local Docker Deployment (Recommended)

```bash
# Build Docker image
docker build -t whatsrook .

# Run container with persistent volume
docker run -d \
  --name whatsrook \
  -p 3000:3000 \
  -e SESSION=1234567890 \
  -e PORT=3000 \
  -v whatsrook_auth:/app/auth \
  whatsrook
```

### 2. Docker Compose Deployment

```bash
SESSION=1234567890 docker compose up -d --build
```

### 3. Heroku Deployment

Deploy WhatsRook directly to Heroku as a Docker container using the **Deploy to Heroku** button or Heroku CLI:

[![Deploy to Heroku](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/Thruqe/whatsrook)

### 4. Render Deployment

Deploy WhatsRook to Render with persistent session volume storage using the **Deploy to Render** button:

[![Deploy to Render](https://render.com/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/Thruqe/whatsrook)

## Contributing

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Disclaimer

See [DISCLAIMER.md](DISCLAIMER.md) for full terms and limitations.

## License

MIT — see [LICENSE](LICENSE) for details.
