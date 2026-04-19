# tech.md

## Stack

- **Language:** Go 1.22
- **HTTP:** Gin (`github.com/gin-gonic/gin`)
- **Persistence:** GORM (`gorm.io/gorm`) + SQLite (`gorm.io/driver/sqlite`). Swappable to Postgres via driver change; Repository port is DB-agnostic.
- **Config:** viper-backed flat struct (`internal/bootstrap/env.go`), env + `.env`.
- **Logging:** `log/slog` via port (`domain/shared.Logger`), adapter in `infrastructure/observability/`.
- **IDs:** `github.com/google/uuid`.
- **Task runner:** Taskfile (`task run`, `task test`, ...).
- **Lint:** golangci-lint with `errcheck`, `gosec`, `govet`, `staticcheck`, `contextcheck`, `ineffassign`, `unused`.

## Planned (later plans)

- LLM: `github.com/anthropics/anthropic-sdk-go` behind `domain/shared.LLMClient`.
- RSS: `github.com/mmcdole/gofeed` behind `domain/article.RSSFetcher`.
- HTML extraction: `github.com/JohannesKaufmann/html-to-markdown`.
- PDF extraction: `github.com/ledongthuc/pdf`.
- API docs: `github.com/swaggo/gin-swagger` + `github.com/swaggo/swag`.

## Testing

- Unit tests colocated (`*_test.go`) next to production files.
- Integration tests under `tests/integration/` with build tag `integration`.
- Hand-written fakes under `tests/mocks/` — no mock-generation tools.
