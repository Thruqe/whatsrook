FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN curl -L https://github.com/Thruqe/whatsrook/releases/download/v4.0.0/whatsrook-linux-amd64.tar.gz -o whatsrook.tar.gz \
    && tar -xzf whatsrook.tar.gz \
    && rm whatsrook.tar.gz

ENV PORT=3000
ENV AUTH_DIR=/app/auth

VOLUME ["/app/auth"]

EXPOSE ${PORT}

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]