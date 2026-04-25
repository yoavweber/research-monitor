# Implementation Plan

- [x] 1. Foundation — paper domain extensions
- [x] 1.1 Extend paper.Entry, introduce the Repository port, define sentinels, delete the speculative UseCase
  - Add a `Source string` field to `paper.Entry` alongside the existing fields.
  - Delete the speculative `paper.UseCase` interface from `domain/paper/ports.go` (arxiv is its only consumer and that consumer is being migrated to an arxiv-application-local `OutcomeFetcher` in a later task).
  - Introduce `paper.Repository` with three methods — `Save(ctx, Entry) (isNew bool, err error)`, `FindByKey(ctx, source, sourceID string) (*Entry, error)`, `List(ctx) ([]Entry, error)` — with the doc-comments from design §Domain Layer, including the `DEDUPE:` marker on `Save` and the explicit note that the repository owns sentinel translation (returns `paper.ErrNotFound` on miss, `paper.ErrCatalogueUnavailable` on any other DB failure).
  - Add two sentinels to `domain/paper/errors.go`: `ErrNotFound` (`shared.NewHTTPError(http.StatusNotFound, "paper not found", nil)`) and `ErrCatalogueUnavailable` (`shared.NewHTTPError(http.StatusInternalServerError, "paper catalogue unavailable", nil)`), mirroring the existing `ErrUpstream*` style.
  - Fixture-style unit tests for the two new sentinels (code / message / `shared.AsHTTPError` round-trip), matching the pattern used in `domain/paper/errors_test.go` today.
  - `go build ./internal/domain/paper/...` succeeds; sentinel tests pass; all downstream callers of the deleted `paper.UseCase` in the current tree surface as clean compile errors (they will be repaired by tasks 5.1, 6.1, 6.2, 7.1).
  - _Requirements: 1.3, 1.4, 1.5, 2.2, 5.4, 5.5_

- [ ] 2. Core — persistence adapter
- [ ] 2.1 (P) Implement paper.Repository on SQLite with composite-unique-index dedupe + sentinel translation
  - New package `internal/infrastructure/persistence/paper/`. Persistence model `Paper` with every domain field mapped; `Authors` and `Categories` stored as JSON-encoded text since SQLite has no native array type; `submitted_at` indexed for the list's ORDER BY.
  - The composite uniqueness invariant is declared via GORM struct tags — `Source` and `SourceID` both carry `uniqueIndex:idx_papers_source_source_id` — so the DB creates the index during `AutoMigrate` and rejects duplicates atomically. The `// DEDUPE:` marker from design §Infrastructure Layer is preserved in the source.
  - `FromDomain` / `ToDomain` round-trip every field including the slices.
  - `repository.Save` calls `db.Create`; on `errors.Is(err, gorm.ErrDuplicatedKey)` it returns `(false, nil)` — the dedupe-skip outcome; on any other DB error it wraps with `paper.ErrCatalogueUnavailable` (using `fmt.Errorf("%w: %v", paper.ErrCatalogueUnavailable, err)` so `errors.Is(err, paper.ErrCatalogueUnavailable)` holds while preserving the underlying message in logs).
  - `repository.FindByKey` returns `(nil, paper.ErrNotFound)` on `gorm.ErrRecordNotFound`, `(*Entry, nil)` on hit, and `(nil, paper.ErrCatalogueUnavailable)` on any other DB failure.
  - `repository.List` orders by `submitted_at DESC` and returns `([]Entry{}, nil)` (non-nil empty slice) when the table is empty; on DB failure wraps with `paper.ErrCatalogueUnavailable`.
  - Register the new model in `internal/infrastructure/persistence/migrate.go` so the table and composite index exist after `persistence.AutoMigrate(db)`.
  - Unit tests against a temp SQLite file cover: new-row save, dedupe-skip on repeated save, same `SourceID` with different `Source` persists twice, `FindByKey` miss → `ErrNotFound`, list ordering by `SubmittedAt DESC`, empty list returns non-nil slice, `ToDomain`/`FromDomain` round-trip preserves `Authors` and `Categories`, **sentinel translation under DB failure** (close the temp DB and assert `Save`/`FindByKey`/`List` all return errors satisfying `errors.Is(err, paper.ErrCatalogueUnavailable)`).
  - `go test ./internal/infrastructure/persistence/paper/...` passes all cases; `AutoMigrate` on a fresh temp DB produces a `papers` table with the named composite index visible via `PRAGMA index_list('papers')`.
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.6, 2.1, 2.2, 3.1, 3.2, 3.3, 4.1, 4.2, 5.5_
  - _Boundary: infrastructure/persistence/paper_
  - _Depends: 1.1_

- [ ] 3. Core — arxiv source stamping
- [ ] 3.1 (P) Stamp Source="arxiv" in the arxiv parser
  - Add an exported constant `SourceArxiv = "arxiv"` in a new file `internal/infrastructure/arxiv/source.go` (package-local; the parser and any future arxiv-side code reference it).
  - Modify `parseFeed` so every constructed `paper.Entry` has `Source = SourceArxiv`. No other change to parser behaviour.
  - Update `parser_test.go` happy-path fixture assertions to additionally check `entries[0].Source == "arxiv"` and `entries[1].Source == "arxiv"`. Empty and error-entry fixtures are unchanged.
  - `go test ./internal/infrastructure/arxiv/...` — parser tests pass with the new assertion; the fetcher's existing tests still pass (they do not inspect `Source`).
  - _Requirements: 5.4_
  - _Boundary: infrastructure/arxiv (parser)_
  - _Depends: 1.1_

- [ ] 4. Core — paper HTTP query layer
- [ ] 4.1 (P) Implement PaperController and its wire DTOs
  - New package `internal/http/controller/paper/` (local import alias `paperctrl` at call sites).
  - `PaperController` holds `paper.Repository` (no application-layer wrapper) and `shared.Clock` (kept for symmetry with the arxiv controller — unused today, no ceremony to remove).
  - `controller.go` exposes `Get(c *gin.Context)` (reads `:source` and `:source_id` path params, calls `repo.FindByKey`, `c.Error(err)` on failure so the existing `ErrorEnvelope` middleware renders the response, `common.Data(ToPaperResponse(entry))` on success) and `List(c *gin.Context)` (calls `repo.List`, error passthrough, `common.Data(ToPaperListResponse(entries))`).
  - `responses.go` declares `PaperResponse` (the 12-field wire shape from design §Interface Layer), `PaperListResponse{ Papers []PaperResponse; Count int }`, and `ToPaperResponse` / `ToPaperListResponse` mappers. `ToPaperListResponse` uses `make([]PaperResponse, 0, len(entries))` so an empty catalogue marshals as `"papers":[]` — not `null`.
  - Unit tests with a fake `paper.Repository` and a fake clock, mounting `middleware.ErrorEnvelope()` on the test engine. Cases: Get hit (JSON contains `source`, `source_id`, all 12 fields); Get miss via `paper.ErrNotFound` (404 envelope); Get failure via `paper.ErrCatalogueUnavailable` (500 envelope); List with two entries returned in the repo's order; List empty asserts the raw-JSON substring `"papers":[]` (guards against nil-vs-empty regression); List failure (500 envelope).
  - `go test ./internal/http/controller/paper/...` passes.
  - _Requirements: 1.4, 2.1, 2.2, 2.4, 3.1, 3.3, 3.5_
  - _Boundary: http/controller/paper_
  - _Depends: 1.1_

- [ ] 5. Core — arxiv ingestion integration
- [ ] 5.1 (P) Modify arxivUseCase to persist + surface is_new outcomes
  - In `internal/application/arxiv/usecase.go`, add:
    - `FetchedEntry struct{ Entry paper.Entry; IsNew bool }` (arxiv-application-specific type — does NOT belong in `domain/paper`).
    - `OutcomeFetcher` interface with `FetchWithOutcomes(ctx) ([]FetchedEntry, error)`.
  - Constructor signature changes to `NewArxivUseCase(fetcher paper.Fetcher, repo paper.Repository, log shared.Logger, query paper.Query) OutcomeFetcher`.
  - `FetchWithOutcomes`: call `fetcher.Fetch(ctx, query)`; iterate returned entries in order; for each, call `repo.Save(ctx, e)`; on success append `FetchedEntry{Entry: e, IsNew: isNew}`; on save failure return `(nil, saveErr)` immediately (no partial slice, R5.5); after the loop emit one `paper.fetch.ok` log with `new` + `skipped` counts; outcomes are returned in the exact order the fetcher produced them (R5.7).
  - Upstream fetch errors propagate unchanged (existing translation in `arxivFetcher` still applies). Save failures already arrive as `paper.ErrCatalogueUnavailable` from the repository — the use case relays them verbatim, no wrapping needed.
  - Update `usecase_test.go`: fake `paper.Fetcher` + fake `paper.Repository`. Cases: happy (3 entries, fake repo always `(true, nil)` → outcomes order+length match, all `IsNew=true`, `paper.fetch.ok` log has `new=3 skipped=0`); mixed outcomes (2 new + 1 skipped, counts match); upstream fetcher error (repo never called, error relayed); save-failure mid-loop on the second of three entries (fake repo returns `paper.ErrCatalogueUnavailable`; `FetchWithOutcomes` returns `(nil, paper.ErrCatalogueUnavailable)`; only one save attempt beyond the first was made; zero outcomes leaked).
  - `go test ./internal/application/arxiv/...` passes.
  - _Requirements: 1.5, 5.1, 5.2, 5.3, 5.5, 5.7_
  - _Boundary: application/arxiv_
  - _Depends: 1.1_

- [ ] 5.2 Modify ArxivController to consume OutcomeFetcher and expose source + is_new on the response
  - Update the constructor to `NewArxivController(uc arxivapp.OutcomeFetcher, clock shared.Clock) *ArxivController`; the controller calls `uc.FetchWithOutcomes(ctx)`.
  - `responses.go`: `EntryResponse` gains `Source string json:"source"` and `IsNew bool json:"is_new"`. `ToFetchResponse` now takes `[]arxivapp.FetchedEntry` (not `[]paper.Entry`) and maps each item onto both new fields plus the pre-existing ones.
  - Update `controller_test.go`: fake `arxivapp.OutcomeFetcher`. New cases: response body includes `source` and `is_new` on each entry; is_new=true and is_new=false mix round-trips through the envelope; `OutcomeFetcher` returning `paper.ErrCatalogueUnavailable` renders a 500 envelope (R5.5 path). Keep the existing happy / empty / sentinel-translation cases green.
  - `go test ./internal/http/controller/arxiv/...` passes.
  - _Requirements: 5.2, 5.3, 5.4, 5.7_
  - _Depends: 5.1_

- [ ] 6. Integration — HTTP route layer update
- [ ] 6.1 Extend route.Deps with PaperConfig, register PaperRouter, rewire ArxivRouter
  - Add `PaperConfig{ Repo paper.Repository }` and a new `Paper PaperConfig` field on `Deps` in `internal/http/route/route.go`.
  - Rewrite `ArxivRouter` so it calls `NewArxivUseCase(d.Arxiv.Fetcher, d.Paper.Repo, d.Logger, d.Arxiv.Query)` — the new `paper.Repository` dependency — and `arxivctrl.NewArxivController(uc, d.Clock)` where `uc` is the `OutcomeFetcher` returned from the use-case constructor.
  - New `paper_route.go` with `PaperRouter(d Deps)` that locally constructs `paperctrl.NewPaperController(d.Paper.Repo, d.Clock)` and registers `GET /papers` → `ctrl.List`, `GET /papers/:source/:source_id` → `ctrl.Get` under `d.Group.Group("/papers")`.
  - `Setup` invokes `PaperRouter(d)` after `ArxivRouter(d)`.
  - Route-level smoke test `paper_route_test.go`: builds an in-memory Gin engine, configures a fake `paper.Repository` via `Deps.Paper.Repo`, issues `GET /api/papers` and `GET /api/papers/arxiv/x`, asserts the handlers are invoked (status 200 / 404 from fake).
  - `go build ./internal/http/...` compiles; `go test ./internal/http/route/...` passes; the pre-existing `arxiv_route_test.go` still passes after the rewiring (its Deps literal must include the new `Paper.Repo` field — adjust as needed).
  - _Requirements: 2.1, 2.3, 3.1, 3.4, 5.2_
  - _Depends: 4.1, 5.1, 5.2_

- [ ] 7. Integration — bootstrap and test harness
- [ ] 7.1 (P) Wire the paper repository pipeline into bootstrap
  - In `internal/bootstrap/app.go`, after `persistence.AutoMigrate(db)` (which now includes the `papers` table):
    - Construct `paperRepo := paperpersist.NewRepository(db)`.
    - Thread `paperRepo` into `route.Deps` as `Paper: route.PaperConfig{Repo: paperRepo}`.
    - The Arxiv config remains the same shape; the use case is built inside `ArxivRouter` now, so the bootstrap layer doesn't construct it.
  - `AutoMigrate` failure surfaces as the existing `fmt.Errorf("migrate: %w", err)` return from `NewApp` — satisfies R4.3 and R4.4 without new bootstrap code.
  - Extend / adapt the existing `app_test.go`: after `LoadEnv` + `NewApp`, a GET on `/api/papers` with a valid token returns 200 with `"papers":[]` `"count":0`; a GET on `/api/papers/arxiv/unknown` returns 404. Auth failure (missing token) is still 401.
  - `go build ./cmd/api` links; `go test ./internal/bootstrap/...` passes.
  - _Requirements: 4.3, 4.4, 5.1_
  - _Boundary: bootstrap_
  - _Depends: 2.1, 3.1, 6.1_

- [ ] 7.2 (P) Extend the integration test harness with a repository injection point
  - In `tests/integration/setup/setup.go`, extend `TestEnvOpts` with a nullable `PaperRepo paper.Repository`. When `nil`, the harness builds a real repository on top of the temp SQLite DB (same DB the harness already sets up for `source` tests). When provided, the harness uses the injected one verbatim — enables failure-injection for R5.5 integration tests.
  - The harness registers `PaperRouter(d)` on the `/api` group (same group that already mounts `ArxivRouter` and the `APIToken` middleware) and threads the repo through as `Deps.Paper.Repo`. The arxiv use case constructed inside `ArxivRouter` automatically receives the same repo via `Deps.Paper.Repo`.
  - Expose the built-or-injected repo on the returned `TestEnv` so tests can verify persisted state directly when useful.
  - New hand-written fake `tests/mocks/paper_repo.go` — implements `paper.Repository`, records `Save` / `FindByKey` / `List` invocations, returns caller-configured `(isNew, err)` tuples per call. Follows the style of the existing `paper_fetcher.go` fake.
  - Harness-level smoke test: `SetupTestEnv(t)` without opts returns a `TestEnv` whose `/api/papers` responds 200 empty; with an injected failing repo (Save returns `paper.ErrCatalogueUnavailable`), `/api/arxiv/fetch` responds 500.
  - `go test -tags=integration ./tests/integration/...` passes the updated harness smoke test alongside every pre-existing integration test.
  - _Requirements: 5.5_
  - _Boundary: tests/integration/setup_
  - _Depends: 2.1, 4.1, 6.1_

- [ ] 8. Validation — endpoint integration tests
- [ ] 8.1 (P) Paper endpoints end-to-end through the real repository
  - New file `tests/integration/papers_test.go` under the `integration` build tag. Uses the default (real-repo) `SetupTestEnv`.
  - Cases: 401 (missing and invalid token) on both endpoints; 404 on `GET /api/papers/arxiv/nonexistent`; list empty returns 200 with `"papers":[]` and `"count":0`; after directly calling `paper.Repository.Save` with a known entry through the harness's exposed repo, `GET /api/papers/arxiv/<source_id>` returns 200 with all 12 fields, and `GET /api/papers` returns a single-item list; saving two entries with different `SubmittedAt` values, list orders them newest-first (R3.2); saving two entries with the same `SourceID` but different `Source` values, both are visible in list and retrievable by their composite key (R1.3).
  - `go test -tags=integration -count=1 ./tests/integration/...` — all paper-endpoint cases pass alongside the pre-existing source and arxiv tests.
  - _Requirements: 1.3, 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 3.4, 3.5_
  - _Boundary: tests/integration (papers)_
  - _Depends: 7.2_

- [ ] 8.2 (P) Arxiv fetch endpoint end-to-end with auto-persist + is_new + save-failure
  - Modify `tests/integration/arxiv_test.go`: augment the existing happy-path test to assert `is_new=true` on every returned entry on first call, `is_new=false` on an immediate second call, and `source="arxiv"` on every entry. After the first call, `GET /api/papers/arxiv/<first_source_id>` returns 200 with the stored entry (proves R5.1 persistence side effect).
  - New case: configure `TestEnvOpts.PaperRepo` with a fake whose `Save` returns `paper.ErrCatalogueUnavailable` → `GET /api/arxiv/fetch` returns HTTP 500 with the standard error envelope (R5.5). The fake records exactly one Save attempt before the short-circuit.
  - Removes any lingering "no datastore writes" assertion if one ever existed — integration coverage of this spec is the authoritative evidence that arxiv-fetcher's requirement 1.4 has been superseded (R5.6).
  - `go test -tags=integration -count=1 ./tests/integration/...` passes all updated arxiv scenarios alongside the pre-existing harness tests.
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7_
  - _Boundary: tests/integration (arxiv)_
  - _Depends: 7.2_

## Implementation Notes

- The `paper.UseCase` interface is **deleted** (not renamed) in task 1.1. After that task compiles, every file that referenced it — `application/arxiv/usecase.go`, `http/controller/arxiv/controller.go`, and both files' tests, plus `http/route/arxiv_route.go` — will fail to compile. Those are repaired in tasks 5.1, 5.2, and 6.1 respectively. The intermediate broken build is expected; `go build ./...` first goes green again at the end of task 6.1.
- There is **no `application/paper/` package**. Sentinel translation lives in the repository (`paper.Repository.Save` wraps non-dedupe errors with `paper.ErrCatalogueUnavailable`; `FindByKey` translates `gorm.ErrRecordNotFound` to `paper.ErrNotFound`). The decision rationale is in research.md §"Drop paper.CatalogueUseCase".
- The composite-unique-index dedupe relies on `errors.Is(err, gorm.ErrDuplicatedKey)`. Verify the SQLite driver actually surfaces this via the direct test in task 2.1 — GORM's contract is driver-agnostic but the surface behaviour is worth a runtime check, not just a compile-time trust.
- The `source` aggregate uses flat-file style (`application/source_usecase.go`, `http/controller/source_controller.go`). This spec adopts the sub-package style for `paper` (matching `arxiv`). If you want layout consistency, a separate refactor PR can promote the source aggregate to sub-packages later — out of scope here.
