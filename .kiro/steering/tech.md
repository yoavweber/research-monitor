# tech.md

## Stack

- **Language:** Go 1.25
- **HTTP:** Gin (`github.com/gin-gonic/gin`)
- **Persistence:** GORM (`gorm.io/gorm`) + SQLite (`gorm.io/driver/sqlite`). Swappable to Postgres via driver change; Repository port is DB-agnostic.
- **Config:** viper-backed flat struct (`internal/bootstrap/env.go`), env + `.env`.
- **Logging:** `log/slog` via port (`domain/shared.Logger`), adapter in `infrastructure/observability/`.
- **IDs:** `github.com/google/uuid`.
- **Task runner:** Taskfile (`task run`, `task test`, ...).
- **Lint:** golangci-lint with `errcheck`, `gosec`, `govet`, `staticcheck`, `contextcheck`, `ineffassign`, `unused`.
- **API docs:** `github.com/swaggo/gin-swagger` + `github.com/swaggo/swag`. Run `task swag` after editing controller annotations. Generated `docs/` is committed. UI mounted at `/swagger/index.html` only when `APP_ENV != prod`.

## Planned (later plans)

- LLM: `github.com/anthropics/anthropic-sdk-go` behind `domain/shared.LLMClient`.
- RSS: `github.com/mmcdole/gofeed` behind `domain/article.RSSFetcher`.
- HTML extraction: `github.com/JohannesKaufmann/html-to-markdown`.
- PDF extraction: `github.com/ledongthuc/pdf`.

## Comments

Default to no comment. Add one only when the *why* is non-obvious — a hidden constraint, a workaround, an invariant the reader can't infer from the code.

Rules:

- **Plain language, short.** Two or three lines max for most. Reserve longer comments for genuine subtlety, not for restating an interface.
- **No restating the code.** If a well-named function says `Ensure(ctx, key) (Locator, error)`, do not write a comment that says "Ensure ensures bytes for key, returning a Locator." Delete it.
- **No exhaustive precondition / postcondition lists** unless an invariant is genuinely surprising and load-bearing. Production Go interfaces document behavior in terms a caller actually needs, not a JSDoc-style spec.
- **No meta-comments.** Don't reference tasks, PRs, the user, prior conversation, or "now we also handle X." The comment must stand alone six months later.
- **Sentinel error lists belong in code, not prose.** If callers need to discriminate, expose typed errors and let `errors.Is` do the talking.

Symptom that a comment block is too heavy: it's longer than the function or interface method it documents. When in doubt, cut it in half and re-read.

## Testing

- Unit tests colocated (`*_test.go`) next to production files.
- Integration tests under `tests/integration/` with build tag `integration`.
- Hand-written fakes under `tests/mocks/` — no mock-generation tools.
