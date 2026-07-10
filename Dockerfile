FROM mcr.microsoft.com/devcontainers/base:bookworm

RUN apt-get update && apt-get install -y \
    ffmpeg \
    libwebp-dev \
    curl \
    bash \
    unzip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:$PATH"

WORKDIR /app

COPY package.json ./
RUN bun install

COPY cli/ ./cli/
COPY lib/ ./lib/
COPY jsconfig.json ./

USER root

CMD ["bun", "start"]
