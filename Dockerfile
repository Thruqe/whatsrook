FROM mcr.microsoft.com/devcontainers/base:bookworm

RUN apt-get update && apt-get install -y \
    ffmpeg \
    libwebp-dev \
    curl \
    bash \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
    -o /usr/local/bin/yt-dlp \
    && chmod a+rx /usr/local/bin/yt-dlp

RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:$PATH"

ENV NVM_DIR="/root/.nvm"
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash \
    && . "$NVM_DIR/nvm.sh" \
    && nvm install node \
    && nvm use node \
    && npm i -g pm2

RUN NODE_VERSION=$(ls /root/.nvm/versions/node/) \
    && ln -sf /root/.nvm/versions/node/$NODE_VERSION/bin/pm2 /usr/local/bin/pm2 \
    && ln -sf /root/.nvm/versions/node/$NODE_VERSION/bin/node /usr/local/bin/node

WORKDIR /app

COPY package.json ./
RUN bun install

COPY client/ ./client/
COPY lib/ ./lib/
COPY jsconfig.json ./

USER root

CMD ["bun", "start"]