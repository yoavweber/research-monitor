# Testing Standards

Project memory for how Go tests are written in this backend. New tests must follow these patterns; older tests that diverge are migrated opportunistically.

## Philosophy

- Test behavior, not implementation.
- Prefer real collaborators with a real (in-memory) database over hand-rolled fakes.
- Keep cases isolated: every test calls `t.Parallel()` and owns its own DB.

## Naming

Top-level test name describes the function under test. Behavior lives in subtest names, not in the top-level identifier.

```go
func TestRepository_Save(t *testing.T) {
    t.Run("persists a new source", func(t *testing.T) { /* ... */ })
    t.Run("returns ErrConflict on duplicate URL", func(t *testing.T) { /* ... */ })
}
```

- Top level: `TestType_Method` or `TestFunction` — what is under test.
- Subtest: a sentence describing expected behavior, lowercase, no underscores.
- Avoid flattening — even single-case tests use `t.Run` so future cases slot in cleanly.

## Structure (AAA)

Table-driven by default. Inside each case, separate Arrange / Act / Assert with **blank lines** — no `// Arrange` comments, the blank lines carry the structure.

```go
func TestSourceUseCase_Create(t *testing.T) {
    tests := []struct {
        name    string
        req     domain.CreateRequest
        wantErr error
    }{
        {
            name: "creates active source with assigned ID",
            req:  domain.CreateRequest{Name: "Test", Kind: domain.KindRSS, URL: "https://example.com/feed.xml"},
        },
        {
            name:    "rejects empty URL",
            req:     domain.CreateRequest{Name: "Test", Kind: domain.KindRSS},
            wantErr: domain.ErrInvalidURL,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            uc, _ := newUseCase(t)

            got, err := uc.Create(context.Background(), tc.req)

            if !errors.Is(err, tc.wantErr) {
                t.Fatalf("err = %v want %v", err, tc.wantErr)
            }
            if tc.wantErr == nil && got.ID == "" {
                t.Error("ID not assigned")
            }
        })
    }
}
```

Single-case tests follow the same blank-line rhythm:

```go
func TestRepository_FindByID(t *testing.T) {
    t.Run("returns ErrNotFound for missing id", func(t *testing.T) {
        t.Parallel()
        repo := sourcerepo.NewRepository(newTestDB(t))

        _, err := repo.FindByID(context.Background(), "missing")

        if !errors.Is(err, domain.ErrNotFound) {
            t.Fatalf("err = %v want ErrNotFound", err)
        }
    })
}
```

## Dependencies — DI First, Real over Fake

Decide in this order. Stop at the first one that fits:

1. **Inject and supply a real value.** If the collaborator can be passed through the constructor and given a real instance (a struct, a pure function, a value), do that. No double, no port, no test fixture.
2. **Inject the port and supply the real implementation.** If the production type already implements an interface, wire the real impl with a test-friendly backend — the canonical case is a `domain.Repository` backed by GORM over in-memory SQLite. This catches schema and migration bugs a fake would silently mask.
3. **Hand-rolled fake (last resort).** Only when steps 1 and 2 don't apply: the collaborator is an out-of-process system with no in-process equivalent (HTTP, S3, exchanges, LLM providers), or the test needs a contract violation the real impl cannot produce. Justify in a comment at the fake's declaration.

Before writing any double, ask: *can I pass a real one through the constructor?* Most of the time the answer is yes and the test gets shorter.

Standard SQLite helper (already in [internal/infrastructure/persistence/source/repo_test.go](backend/internal/infrastructure/persistence/source/repo_test.go)):

```go
func newTestDB(t *testing.T) *gorm.DB {
    t.Helper()
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    if err != nil { t.Fatalf("open: %v", err) }
    if err := persistence.AutoMigrate(db); err != nil { t.Fatalf("migrate: %v", err) }
    return db
}
```

## Test Doubles — Single Home

All hand-written mocks/fakes/stubs for non-DB collaborators live in [backend/tests/mocks/](backend/tests/mocks/). Do not create test doubles inside `_test.go` files or per-package helpers.

- One file per port: `tests/mocks/paper_fetcher.go` implements `paper.Fetcher`.
- Doubles are concurrency-safe (`sync.Mutex`) and record calls so tests can assert invocation count and arguments.
- Zero value must be usable — configuration is via exported fields, not constructors.

See [backend/tests/mocks/paper_fetcher.go](backend/tests/mocks/paper_fetcher.go) for the canonical shape.

## Test Types & Layout

| Type | Location | Build tag |
|---|---|---|
| Unit | co-located `*_test.go` next to the package | none |
| Integration | [backend/tests/integration/](backend/tests/integration/) | `//go:build integration` |
| Manual / live | [backend/tests/manual/](backend/tests/manual/) | `//go:build manual` |

- Unit tests use `package foo_test` (black-box) by default; switch to `package foo` only when testing unexported helpers.
- Integration tests boot the real HTTP server via `setup.SetupTestEnv(t)` and exercise routes end-to-end against in-memory SQLite.
- Manual tests hit live external systems and never run in CI.

## Determinism

- Inject a clock (`shared.Clock`) — never call `time.Now()` from domain or application code under test.
- Use `t.TempDir()` for any filesystem state; rely on `t.Cleanup` rather than `defer` for teardown that must survive panics.
- Every test calls `t.Parallel()` unless it mutates a process-global (env vars, working dir) — and those should be refactored, not whitelisted.

---
_Patterns only. Tool config (gotestsum, coverage thresholds) belongs in tech.md or CI._
