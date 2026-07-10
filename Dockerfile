FROM mcr.microsoft.com/devcontainers/base:noble
RUN apt-get update && apt-get install -y \
    ffmpeg \
    libwebp-dev \
    curl \
    bash \
    unzip \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL https://bun.sh/install | bash
RUN curl -L https://github.com/zevlion/rpm2/releases/download/latest/rpm2 -o /usr/local/bin/rpm2 && \
    chmod +x /usr/local/bin/rpm2
ENV PATH="/root/.bun/bin:$PATH"
WORKDIR /app
COPY package.json ./
RUN bun install
COPY web.mts ./
COPY src/ ./src/
COPY web/ ./web/
COPY tsconfig.json ./
RUN bun run build
USER root
CMD ["bun", "run", "web"]
