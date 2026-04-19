# DeFi Research Monitor — Backend

Personal research-feed backend. Aggregates DeFi news + academic sources, summarises via LLM, exposes a chronological feed.

## Run

```bash
cp .env.example .env
# edit .env: set API_TOKEN and (later) ANTHROPIC_API_KEY
task run
```

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
