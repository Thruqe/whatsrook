FROM debian:bookworm-slim

ARG TARGETARCH=amd64

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    tar \
    gzip \
    ffmpeg \
    python3 \
    && rm -rf /var/lib/apt/lists/* \
    && curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp

WORKDIR /app

RUN curl -L "https://github.com/Thruqe/whatsrook/releases/download/alpha/whatsrook-linux-${TARGETARCH}.tar.gz" | tar -xz -C /app \
    && chmod +x /app/whatsrook

ENTRYPOINT ["/app/whatsrook"]
