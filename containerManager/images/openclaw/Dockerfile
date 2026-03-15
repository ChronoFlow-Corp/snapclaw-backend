FROM node:22-bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends git ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

RUN corepack enable

RUN npm i -g openclaw@latest

ENV NODE_ENV=production
ENV OPENCLAW_HOME=/app/.openclaw
WORKDIR /app

RUN mkdir -p /app/.openclaw && chown -R node:node /app
USER node

ENTRYPOINT ["openclaw", "gateway"]