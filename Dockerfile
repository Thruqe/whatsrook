FROM mcr.microsoft.com/devcontainers/base:noble
RUN apt-get update && apt-get install -y \
    curl \
    bash \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://raw.githubusercontent.com/zevlion/rpm/refs/heads/master/scripts/linux-installer.sh?$(date +%s)" | bash
WORKDIR /app
CMD ["bash"]
