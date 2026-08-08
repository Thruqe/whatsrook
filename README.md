# WhatsRook

> [!CAUTION]
> Educational project only. See [DISCLAIMER.md](DISCLAIMER.md) before use.

Read [Documentation](https://github.com/Thruqe/whatsrook-docs) for detailed architecture, CLI usage, WebSocket IPC specification, plugin development, and deployment guides.

Connect your app to WhatsApp and receive live events — messages, groups, stories, channels — then send actions back programmatically.

[![Go Code Quality & Tests](https://img.shields.io/github/actions/workflow/status/Thruqe/whatsrook/go-checks.yml?style=for-the-badge&logo=github&label=Go%20Checks)](https://github.com/Thruqe/whatsrook/actions/workflows/go-checks.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26.4-blue?style=for-the-badge&logo=go&logoColor=white)](https://github.com/Thruqe/whatsrook/blob/master/go.mod)
[![Release](https://img.shields.io/badge/Release-v4.0.0-orange?style=for-the-badge&logo=github)](https://github.com/Thruqe/whatsrook/releases)
[![License](https://img.shields.io/badge/License-MIT-yellow?style=for-the-badge)](LICENSE)
[![Join WhatsApp Channel](https://img.shields.io/badge/WhatsApp-Channel-25D366?style=for-the-badge&logo=whatsapp&logoColor=white)](https://whatsapp.com/channel/0029Vb8Vo0k0bIdsTOTF1G2o)
[![Join Telegram Channel](https://img.shields.io/badge/Telegram-Channel-26A5E4?style=for-the-badge&logo=telegram&logoColor=white)](https://t.me/whatsrook)
[![Supabase DB](https://img.shields.io/badge/Database-Supabase%20PostgreSQL-3ECF8E?style=for-the-badge&logo=supabase&logoColor=white)](https://supabase.com)

## Features

- Real-time event streaming (messages, groups, stories, channels)
- Bidirectional communication — receive events, dispatch actions
- Build bots, automations, and integrations on top of WhatsApp
- Powered by hypermeow (no browser automation, no Puppeteer)

## Database & Storage

WhatsRook uses PostgreSQL as its primary database engine, with automatic fallback to embedded SQLite.

[![Get Free PostgreSQL on Supabase](https://img.shields.io/badge/Get%20Free%20PostgreSQL-Supabase-3ECF8E?style=for-the-badge&logo=supabase&logoColor=white)](https://supabase.com)

Get a free managed PostgreSQL database at [Supabase](https://supabase.com) and set `DATABASE_URL` in your `.env` file. For details, see the [Database & Storage Guide](Documentation/DATABASE.md).

## Deployment

WhatsRook supports multiple deployment platforms including Pterodactyl, Heroku, Render, and Local Docker.

[![Deployment Guide](https://img.shields.io/badge/Read-Deployment%20Guide-blue?style=for-the-badge&logo=readme&logoColor=white)](Documentation/DEPLOYMENT.md)

For step-by-step guides on deploying WhatsRook, see the [Deployment Documentation](Documentation/DEPLOYMENT.md).

## Contributing

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Disclaimer

See [DISCLAIMER.md](DISCLAIMER.md) for full terms and limitations.

## License

MIT — see [LICENSE](LICENSE) for details.
