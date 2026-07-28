FROM node:22-bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends git ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y \
    curl ca-certificates jq \
    && rm -rf /var/lib/apt/lists/* \

RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

RUN corepack enable

RUN curl -L -o gog.tar.gz \
 https://github.com/steipete/gogcli/releases/download/v0.12.0/gogcli_0.12.0_linux_$(dpkg --print-architecture).tar.gz \
 && tar -xzf gog.tar.gz \
 && mv gog /usr/local/bin/gog \
 && rm gog.tar.gz

RUN npm i -g openclaw@latest


ENV NODE_ENV=production
ENV OPENCLAW_HOME=/app/.openclaw
WORKDIR /app

RUN mkdir -p /app/.openclaw && chown -R node:node /app
USER node

ENTRYPOINT ["openclaw", "gateway"]
