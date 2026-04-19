# structure.md

Steering document. Read before every task. Declarative rules only.

## 1. Project shape

- Single Go module, `go.mod` at repo root.
- Top-level directories: `cmd/`, `internal/`, `pkg/`, `tests/`, `docs/`.
- `internal/` holds all production code. `cmd/<binary>/main.go` is the only code outside `internal/` and `pkg/`.

## 2. Layers

```
internal/
├── domain/            # entities + ports (abstractions)
├── application/       # use-case implementations
├── infrastructure/    # outbound adapters (GORM, LLM, RSS, ...)
├── interface/         # inbound adapters (HTTP)
└── bootstrap/         # composition root
```

Dependency rule: inward only.

| Layer | May import |
|---|---|
| `domain/` | stdlib, other `domain/` subpackages |
| `application/` | `domain/`, `pkg/` |
| `infrastructure/` | `domain/`, `pkg/` |
| `interface/` | `domain/`, `application/`, `pkg/` |
| `bootstrap/` | everything |

Forbidden: `domain/` → `infrastructure/persistence/`. Conversion goes through `ToDomain()` / `FromDomain()` on the persistence side.

## 3. Domain subpackage shape

Every aggregate lives at `internal/domain/<entity>/`. Minimum files:

- `model.go` — entities, value objects.
- `ports.go` — `UseCase` and `Repository` interfaces.

Additional as needed:

- `requests.go` — inbound DTOs with `Validate() error`.
- `responses.go` — outbound DTOs.
- `errors.go` — aggregate sentinels.

Cross-cutting ports (`Logger`, `Clock`, `LLMClient`, `Extractor`, `APIFetcher`) live in `internal/domain/shared/ports.go`.

## 4. Ports and implementations

| Interface | Defined in | Implemented in |
|---|---|---|
| `<Entity>UseCase` | `domain/<entity>/ports.go` | `application/<entity>_usecase.go` |
| `<Entity>Repository` | `domain/<entity>/ports.go` | `infrastructure/persistence/<entity>/repo.go` |
| Cross-cutting ports | `domain/shared/ports.go` | `infrastructure/<area>/` |

## 5. Composition

- `bootstrap/app.go` is the only place concrete types are instantiated.
- Route files receive shared infra via `route.Deps`; they build their own `repo → usecase → controller` chain locally.
- No DI framework. Manual wiring only.

## 6. HTTP layer (Gin)

- Gin engine built in `bootstrap/app.go`, passed to `interface/http/route.Setup(...)`.
- One `XxxRouter(d Deps)` function per resource in `interface/http/route/`.
- Middleware applied at group level in `Setup`.
- Controllers bind JSON directly into domain request DTOs.

## 7. Naming

| Thing | Pattern |
|---|---|
| Interface | `UseCase`, `Repository` (scoped by package — refer to them as `source.UseCase`, `source.Repository`) |
| Implementing struct | unexported: `sourceUseCase`, `repository` (or equivalent) |
| Constructor | `NewSourceUseCase(...)` / `NewRepository(...)` returns the interface |

Rationale: Go style discourages repeating the package name in type names. `source.UseCase` is idiomatic; `source.SourceUseCase` stutters.

## 8. Error handling

- `domain/shared/errors.go` defines `HTTPError{Code int, Message string, Err error}` with `Error()` and `Unwrap()`.
- Aggregate-specific sentinels in `domain/<entity>/errors.go`.
- One Gin middleware translates errors to the response envelope (`interface/http/common/`).

## 9. Logging

- `log/slog` only, via the `domain/shared.Logger` port. Adapter in `infrastructure/observability/`.

## 10. Context propagation

`context.Context` is the first parameter of every use-case method, every repository method, every outbound adapter call.

## 11. Configuration

viper-backed flat struct in `bootstrap/env.go`. Fields tagged `mapstructure:"ENV_VAR_NAME"`. Loaded once at startup, passed by pointer.

## 12. Testing

- `tests/integration/` — cross-package integration tests (build tag `integration`).
- `tests/mocks/` — all hand-written fakes.
- Unit tests colocated (`*_test.go`).
- Every test calls `t.Parallel()`.

## 13. Deferred

- Postgres, Redis, job queue, observability backend, email ingestion, API fetcher concrete impls.
