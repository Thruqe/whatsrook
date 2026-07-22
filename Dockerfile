FROM golang:1.26-bookworm AS builder

WORKDIR /app
COPY . .
RUN go build -o whatsrook .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    git \
    tar \
    gzip \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/whatsrook /app/whatsrook
COPY version.toml /app/version.toml
COPY scripts /app/scripts
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENV PORT=3000
ENV AUTH_DIR=/app/auth

VOLUME ["/app/auth"]

EXPOSE ${PORT}

ENTRYPOINT ["/entrypoint.sh"]