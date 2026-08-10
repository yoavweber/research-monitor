# DeFi Research Monitor — Backend

Personal research-feed backend. Aggregates DeFi news + academic sources, summarises via LLM, exposes a chronological feed.

## Run

```bash
cp .env.example .env
# edit .env: set AUTH_JWT_SECRET (openssl rand -base64 32) and (later) ANTHROPIC_API_KEY
task run
go run ./cmd/seed users you@example.com <a-password-12-72-bytes>
```

Then log in: `POST /auth/login` with that email/password returns a 24h JWT; send it as `Authorization: Bearer <token>` on every `/api/*` request. No refresh token — re-run login when it expires.

## Architecture

See [structure.md](structure.md). Product spec in [product.md](product.md). cc-sdd spec workflow lives in `.kiro/` and `.claude/skills/`.

## Commands

See `Taskfile.yml`. Common:

| Command | Purpose |
|---|---|
| `task run` | run the API locally |
| `task test` | unit tests |
| `task test:int` | integration tests |
| `task lint` | golangci-lint |
| `task db:reset` | wipe SQLite and restart |
