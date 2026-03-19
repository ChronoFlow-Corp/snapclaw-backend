# snapclaw-backend

## Deploy (Docker Compose)

### Prerequisites

- Docker and Docker Compose installed.
- A host directory for OpenClaw configs and SimpleClaw backups.

### Prepare host directories (example paths)

Create directories that will be bind-mounted so containerManager can mount configs into OpenClaw containers and simpleClaw can write archives. Example:

```bash
mkdir -p /opt/snapclaw/claw-configs
mkdir -p /opt/snapclaw/simpleclaw-backups
chmod -R 775 /opt/snapclaw/claw-configs
chmod -R 775 /opt/snapclaw/simpleclaw-backups
```

### Update configs

Update these files:

- `deploy/containerManager/config.yaml`
  - `image.base_path` must be an absolute host path, for example:
    - `/opt/snapclaw/claw-configs`
- `deploy/simpleClaw/config.yaml`
  - `hosting.container_manager.backup_path` should point to the container path:
    - `/data/simpleclaw/backups`
  - Replace all `REPLACE_ME` placeholders (Google OAuth, JWT secrets, OpenRouter key).

### Update docker-compose mounts

Ensure the compose file mounts host paths (example):

```
containermanager:
  volumes:
    - /opt/snapclaw/claw-configs:/opt/snapclaw/claw-configs

simpleclaw:
  volumes:
    - /opt/snapclaw/simpleclaw-backups:/data/simpleclaw/backups
```

### Run

```bash
docker compose up --build
```

### Access

- Grafana: http://localhost:3000 (admin/admin)
- Loki: http://localhost:3100
- Prometheus: http://localhost:9090
- Tempo: http://localhost:3200
- simpleClaw API: http://localhost:1337
- containerManager API: http://localhost:8080

### Troubleshooting

If stop/delete fails with `broken pipe` and `Permission denied` inside simpleClaw, ensure the backups directory is writable by the container user or run simpleClaw as root.
