# DeFi Research Monitor — Backend

Personal research-feed backend. Aggregates DeFi news + academic sources, summarises via LLM, exposes a chronological feed.

## Run

```bash
cp .env.example .env
# edit .env: set AUTH_JWT_SECRET (openssl rand -base64 32) and (later) ANTHROPIC_API_KEY
task run                                                    # in one terminal
task seed:user -- you@example.com a-password-12-72-bytes    # one-time, idempotent
task dev:login -- you@example.com a-password-12-72-bytes    # writes .dev-token
```

Every `/api/*` request then needs `Authorization: Bearer $(cat .dev-token)`, e.g.:

```bash
curl -H "Authorization: Bearer $(cat .dev-token)" http://localhost:8080/api/sources
```

The token is a 24h JWT with no refresh — once it expires, re-run `task dev:login`.

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
