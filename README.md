# snapclaw-backend

## Local Dev With Docker Compose

### Minimal setup

1. Copy `.env.example` to `.env`.
2. Fill in `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`.
3. Register this Google OAuth redirect URL:
   - `http://localhost:1337/auth/connect/google/callback`
4. Run `task up`.

### What starts by default

- `simpleclaw` API on `http://localhost:1337`
- `containermanager` API on `http://localhost:8080`
- `simpleclaw-db` on `localhost:5433`
- `containermanager-db` on `localhost:5434`

### Dev defaults

- `simpleClaw` derives local callback URLs from `DEV_HOST` and defaults it to `localhost`.
- JWT RSA keys are generated automatically on first start into [`deploy/simpleClaw/keys`](/Users/kodokuus/work/go/snapclaw-backend/deploy/simpleClaw/keys).
- OpenRouter is optional for local boot. Google OAuth works without it, but creating a `claw` still requires `OPENROUTER_API_TOKEN`.

### Observability

- Basic local dev does not start Grafana, Prometheus, Loki, Tempo, or Alloy.
- Start the full stack with `task up-obs`.

### Useful commands

- `task up`
- `task up-obs`
- `task down`
- `task logs`
- `task logs-obs`
- `task ps`
