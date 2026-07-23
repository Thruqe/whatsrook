# WhatsRook

> [!CAUTION]
> Educational project only. See [DISCLAIMER.md](DISCLAIMER.md) before use.

Real-time WhatsApp API built on [whatsmeow](https://github.com/tulir/whatsmeow).

Connect your app to WhatsApp and receive live events — messages, groups, stories, channels — then send actions back programmatically.

[![Go Code Quality & Tests](https://github.com/Thruqe/whatsrook/actions/workflows/go-checks.yml/badge.svg)](https://github.com/Thruqe/whatsrook/actions/workflows/go-checks.yml)
[![Go Version](https://badgen.net/badge/Go/1.26.4/blue)](https://github.com/Thruqe/whatsrook/blob/master/go.mod)
[![Release](https://badgen.net/badge/Release/v4.0.0/orange)](https://github.com/Thruqe/whatsrook/releases)
[![License](https://badgen.net/badge/License/MIT/yellow)](LICENSE)




## Features

- Real-time event streaming (messages, groups, stories, channels)
- Bidirectional communication — receive events, dispatch actions
- Build bots, automations, and integrations on top of WhatsApp
- Powered by whatsmeow (no browser automation, no Puppeteer)

## Deployment

### 1. Heroku Deployment

Deploy WhatsRook directly to Heroku as a Docker container using the **Deploy to Heroku** button or Heroku CLI:

[![Deploy to Heroku](https://www.herokucdn.com/deploy/button.svg)](https://heroku.com/deploy?template=https://github.com/Thruqe/whatsrook)

#### Manual Heroku CLI Deployment:
```bash
heroku login
heroku container:login
heroku create your-whatsrook-app
heroku stack:set container -a your-whatsrook-app
heroku config:set SESSION=1234567890 -a your-whatsrook-app
heroku container:push web -a your-whatsrook-app
heroku container:release web -a your-whatsrook-app
```

---

### 2. Render Deployment

Deploy WhatsRook to Render with persistent session volume storage using the **Deploy to Render** button:

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/Thruqe/whatsrook)

Render automatically parses [`render.yaml`](./render.yaml) to build the Docker container and attach persistent volume storage at `/app/auth`.

---

### 3. Local Docker Deployment

You can deploy WhatsRook locally or on a private VPS using Docker or Docker Compose.

#### Using Docker Compose (Recommended):
```bash
# Set your SESSION phone number in docker-compose.yml or as an env var
SESSION=1234567890 docker compose up -d --build
```

#### Using Docker CLI:
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

## Contributing

Please read our [Code of Conduct](CODE_OF_CONDUCT.md) before contributing.

## Disclaimer

See [DISCLAIMER.md](DISCLAIMER.md) for full terms and limitations.

## License

MIT — see [LICENSE](LICENSE) for details.
