# Research & Design Decisions — paper-persistence

## Summary

- **Feature**: `paper-persistence`
- **Discovery Scope**: Extension (adds a new aggregate-level capability and modifies the arxiv-fetcher integration seam within an established hexagonal backend).
- **Key Findings**:
  - The `source` aggregate (`internal/infrastructure/persistence/source/{model.go, repo.go}`) is a complete working precedent for a GORM + SQLite repository with `ToDomain`/`FromDomain` conversion, unexported struct, interface-returning constructor, and auto-migration wired via `persistence.AutoMigrate(db)`. Mirroring it keeps the two aggregates structurally identical so a reviewer can read one and understand the other.
  - The existing `paper.UseCase` interface (from `arxiv-fetcher`) has a single method `Fetch(ctx) ([]paper.Entry, error)` — it is the ingestion-side use case. Adding save/get/list to the same interface would merge two orthogonal responsibilities (upstream fetch vs. catalogue management). The cleanest split is to rename the existing interface to `paper.FetchUseCase` and introduce a separate `paper.CatalogueUseCase` for the persistence operations; both carry the `UseCase` suffix per steering §7.
  - GORM supports the required composite unique index via struct tag `gorm:"uniqueIndex:idx_papers_source_source_id"` on both `Source` and `SourceID` columns. Auto-migration creates the index; runtime inserts that violate it surface as `gorm.ErrDuplicatedKey` (or the dialect-specific sqlite wrapping of `UNIQUE constraint failed`), which the repository maps to the "skipped" save outcome. No hand-written migrations are needed.
  - The arxiv-fetcher PR (open at the time of writing) includes `paper.Entry` with no `Source` field. Adding `Source` is a source-compatible additive change: every existing test constructs entries via literal initializer or XML fixture; both forms accept the new field as zero-value or as `"arxiv"` respectively. The parser at `internal/infrastructure/arxiv/parser.go` is the natural place to set `Source = "arxiv"` on every parsed entry because it is already arxiv-specific.

## Research Log

### Composite-key dedupe on SQLite via GORM

- **Context**: Requirement 4 mandates that uniqueness on `(Source, SourceID)` is enforced at the storage layer and survives concurrent save races.
- **Sources Consulted**:
  - Existing `source` aggregate repository: `internal/infrastructure/persistence/source/{model.go, repo.go}` (working precedent for single-column unique index).
  - [GORM Indexes documentation](https://gorm.io/docs/indexes.html) — multi-field composite indexes via `uniqueIndex:idx_name` on multiple fields.
  - SQLite constraint error surface in the `gorm.io/driver/sqlite` wrapper (ErrDuplicatedKey is returned for uniqueness violations in GORM v1.25+).
- **Findings**:
  - Composite unique index syntax: `Source string gorm:"type:text;not null;uniqueIndex:idx_papers_source_source_id"` plus `SourceID string gorm:"type:text;not null;uniqueIndex:idx_papers_source_source_id"`.
  - GORM returns `gorm.ErrDuplicatedKey` when an insert violates the unique constraint (driver-agnostic across sqlite / postgres / mysql). The repository uses `errors.Is(err, gorm.ErrDuplicatedKey)` to branch from "insert failed" to "skip (already present)".
  - Auto-migration creates the index on first run; downstream migration logic is unchanged — the existing `persistence.AutoMigrate(db)` gains one additional model registration.
- **Implications**:
  - Dedupe is enforced by storage, not by a pre-read check, closing the concurrent-save gap required by R4.2.
  - Save returns `(isNew bool, err error)`: `isNew=true` on successful insert, `isNew=false` when `errors.Is(err, gorm.ErrDuplicatedKey)` (and `err=nil` in that case — the skip is a normal outcome, not an error).

### Integration seam between arxiv-fetcher and the catalogue

- **Context**: Requirement 5 couples the arxiv fetch endpoint to the save path. The choice of where to invoke `Save` ripples into application-layer dependencies and the response DTO shape.
- **Sources Consulted**:
  - Existing arxiv use case: `internal/application/arxiv/usecase.go` (currently depends on `paper.Fetcher` + `shared.Logger` + `paper.Query`).
  - Existing arxiv controller + response DTO: `internal/http/controller/arxiv/{controller.go, responses.go}`.
  - The `parser.go` at `internal/infrastructure/arxiv/` — the natural place to stamp `Source="arxiv"`.
- **Findings**:
  - Placing save inside `arxivUseCase` rather than inside the controller keeps the controller as a thin HTTP-shape-only adapter. The use case already owns the outcome log; extending it with save + per-entry `is_new` is additive.
  - The return type of `arxivUseCase.Fetch` changes from `[]paper.Entry` to a new local struct `[]FetchedEntry{Entry paper.Entry; IsNew bool}` (or similar). The controller maps this to the response DTO.
  - `Source` is stamped inside `parser.go` (using an exported constant `SourceArxiv = "arxiv"`). The use case does not need to set it — the domain value arrives already populated.
- **Implications**:
  - `arxivUseCase` gains a dependency on `paper.CatalogueUseCase` (or `paper.Repository` directly — the choice is covered below).
  - Response DTO `EntryResponse` gains `Source string json:"source"` and `IsNew bool json:"is_new"`.
  - Arxiv-fetcher requirement 1.4 is repealed as part of this spec. That spec's integration tests do not currently assert "no DB writes" directly; they assert fake-fetcher invocation counts. No test inversions are needed; new tests are added instead.

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| UseCase layer + Repository layer (mirror `source`) | Application-layer `CatalogueUseCase` wraps a domain-layer `Repository` port; controller depends on the use case. | Matches established precedent exactly; centralises outcome logging in the use case; error translation (ErrNotFound → 404) stays at a consistent layer. | Slight ceremony for a pure-passthrough save method. | Selected. |
| Controller depends on Repository directly | Skip the use-case layer; controller calls repository methods. | Fewer files, fewer tests. | Breaks consistency with `source` aggregate; scatters logging and error-mapping concerns into HTTP-specific code. | Rejected. |
| Merge catalogue into the existing `paper.UseCase` | Add Save/Get/List methods to the existing interface. | No new interface name to invent. | Mixes "fetch from upstream" (ingestion) with "read/write catalogue" (storage), two orthogonal responsibilities in one type. Grows the interface without clear cohesion. | Rejected. |

## Design Decisions

### Decision: Split `paper.UseCase` into `paper.FetchUseCase` (existing, renamed) and `paper.CatalogueUseCase` (new)

- **Context**: The existing `paper.UseCase` carries a single method `Fetch(ctx) ([]Entry, error)` wired up by arxiv-fetcher. Adding save/get/list to it would mix ingestion and storage concerns.
- **Alternatives Considered**:
  1. Merge Save/Get/List into the existing `paper.UseCase` — rejected; violates single-responsibility cohesion.
  2. Introduce `paper.Catalogue` without a `UseCase` suffix — rejected; violates steering §7 naming (UseCase is the established interface suffix at the application-layer entry-point port).
  3. Put the catalogue port in a separate aggregate (`domain/catalogue/`) — rejected; a paper's catalogue IS the paper domain, not a parallel aggregate.
- **Selected Approach**: Rename the existing `paper.UseCase` to `paper.FetchUseCase` and introduce `paper.CatalogueUseCase`. Both retain the `UseCase` suffix and coexist in `domain/paper/ports.go`.
- **Rationale**: Preserves the arxiv-fetcher ingestion use case's identity, makes the catalogue concern a first-class citizen, and respects the naming convention (two explicit `…UseCase` interfaces rather than one overloaded one).
- **Trade-offs**: One breaking rename inside `domain/paper`, `application/arxiv`, and `http/route/arxiv_route.go`. All rename sites are small and local.
- **Follow-up**: None.

### Decision: Drop `paper.CatalogueUseCase`; consumers depend on `paper.Repository` directly

- **Context**: An earlier draft of this design introduced `paper.CatalogueUseCase` between the controllers / arxiv use case and `paper.Repository`. The proposed responsibilities of that layer were per-call save logging, sentinel translation (`gorm.ErrXxx` → `paper.ErrCatalogueUnavailable`), and consistency with the `source` aggregate's `source.UseCase` shape.
- **Alternatives Considered**:
  1. Keep `paper.CatalogueUseCase` as a wrapper over the repository — rejected; on close inspection the layer is a pure pass-through. Per-save logging is redundant given the aggregate `paper.fetch.ok` log emitted by the arxiv use case (with `new` and `skipped` counts). Sentinel translation can move into the repository, mirroring `source.Repository`'s convention of returning `source.ErrNotFound` directly. Pattern consistency with `source.UseCase` only holds when the use case does real work — `source.UseCase.Create` validates requests, mints UUIDs, stamps timestamps, and checks conflicts, none of which apply here.
  2. Add real work to `CatalogueUseCase.Save` (e.g. domain validation that `Source != ""`) to justify the layer — rejected; that's adding code for the sake of justifying a layer rather than because the work is needed.
- **Selected Approach**: Delete `paper.CatalogueUseCase`. The repository owns sentinel translation: `Save` wraps non-dedupe errors with `paper.ErrCatalogueUnavailable`, `FindByKey` wraps non-`gorm.ErrRecordNotFound` errors the same way (and surfaces `paper.ErrNotFound` directly on miss), `List` wraps any error. `arxivUseCase` and `PaperController` consume `paper.Repository` directly. Per-save log lines vanish; the aggregate `paper.fetch.ok` log remains the authoritative outcome record.
- **Rationale**: Honest to the abstraction. A pass-through use-case layer is ceremony; eliminating it reduces files, tests, and dependency edges without losing information. The `source.Repository` precedent already supports repository-side sentinel translation.
- **Trade-offs**: The arxiv use case is now the only place "save outcome went wrong" can be observed at the application layer (via the returned sentinel). For a personal-tool / single-user backend, this is sufficient. If a future spec wants per-save business logging or cross-aggregate orchestration around save, the use-case layer can be re-introduced cheaply at that point.
- **Follow-up**: Revisit if a future spec needs to wrap a save call with non-trivial application logic (transaction, multi-aggregate update, retry policy).

### Decision: `Source` is stamped in the arxiv parser, not in the use case

- **Context**: Every persisted `paper.Entry` must carry `Source="arxiv"` for the arxiv ingestion path. The stamp can happen in three places: the parser, the infrastructure `arxivFetcher`, or the application-layer `arxivUseCase`.
- **Alternatives Considered**:
  1. Stamp in `arxivUseCase` — rejected; forces the application layer to know about source naming.
  2. Stamp in `arxivFetcher` — rejected; that struct composes the parser but doesn't decode the XML itself, so "enrich each entry with a constant" would be a second traversal of the slice.
- **Selected Approach**: `parseFeed` in `internal/infrastructure/arxiv/parser.go` sets `Entry.Source = SourceArxiv` on every entry it returns. `SourceArxiv` is an exported constant in the same package.
- **Rationale**: The parser is already the sole place that constructs `Entry` values from raw arxiv XML. Stamping there keeps source-awareness at the one place that knows "this is an arxiv feed".
- **Trade-offs**: Parser is no longer a pure translation of XML → Entry, in the sense that it also injects the constant. Mitigated by the constant being source-local.
- **Follow-up**: Any future non-arxiv feed implementation does the same in its own parser with its own source constant.

### Decision: Save outcome surfaced as `(isNew bool, err error)`; dedupe-skip is `(false, nil)`, not an error

- **Context**: The caller (arxivUseCase) must distinguish "newly inserted" from "already present" to fill the `is_new` response field. It must also distinguish both of those from a genuine storage failure (R5.5).
- **Alternatives Considered**:
  1. Return an enum / sentinel: `Save returns (SaveOutcome, error)` with `OutcomeInserted | OutcomeSkipped`. Rejected as over-engineered for two cases.
  2. Treat skip as an error sentinel (`ErrDuplicate`) — rejected; duplicate is a normal, expected outcome, not a failure.
- **Selected Approach**: `Save(ctx, entry) (isNew bool, err error)` where `err` is non-nil only for genuine storage failures. Dedupe-skip is `(false, nil)`.
- **Rationale**: Two-case surface is idiomatic Go for "did you actually do something?" questions. Doesn't conflate with errors-as-control-flow.
- **Trade-offs**: None.
- **Follow-up**: None.

### Decision: Catalogue save-failure surfaces through a new `paper.ErrCatalogueUnavailable` sentinel (HTTP 500)

- **Context**: R5.5 requires that if the save path fails (other than dedupe-skip), the arxiv fetch request must return 5xx. The existing `paper.*` sentinels target upstream-fetch failures (502/504), which are semantically wrong for an internal-storage failure.
- **Alternatives Considered**:
  1. Return a raw GORM error upward — rejected; the controller's error envelope middleware would render a generic 500 without a stable message, and the domain layer would leak an infrastructure type.
  2. Reuse `paper.ErrUpstreamUnavailable` (504) — rejected; the failure is internal, not upstream.
- **Selected Approach**: Define `paper.ErrCatalogueUnavailable = shared.NewHTTPError(http.StatusInternalServerError, "paper catalogue unavailable", nil)` as a new domain sentinel. `CatalogueUseCase.Save` wraps any non-duplicate error from the repository with this sentinel.
- **Rationale**: Keeps failure categorisation uniform at the `*shared.HTTPError` sentinel layer that the error-envelope middleware already understands. Operators see "paper catalogue unavailable" in the response, which is actionable.
- **Trade-offs**: One new sentinel to keep in sync across tests.
- **Follow-up**: None.

## Risks & Mitigations

- **Risk**: The cross-cutting change to `arxiv-fetcher`'s use case + controller + parser is not purely additive — the arxiv response DTO gains two fields. Any client that pins the exact shape (strict JSON) would break. — **Mitigation**: The only client today is the integration test harness; additive JSON fields are safe for permissive decoders (maps, soft structs). Not a concern for v1. Call it out in the response shape section of design.md.
- **Risk**: Rename of `paper.UseCase` → `paper.FetchUseCase` touches arxiv-fetcher code landed via another (earlier) PR. If the arxiv-fetcher PR has not yet merged when this spec's work starts, branch-level conflicts are likely. — **Mitigation**: This spec's PR depends on `arxiv-fetcher` having merged first; branch from the updated main. Declared explicitly in the tasks phase.
- **Risk**: Concurrent `GET /api/arxiv/fetch` calls against the same fresh paper would race. The unique index guarantees at-most-one row; the response's `is_new` flag might then be inconsistent between the two racers (both can't report true). — **Mitigation**: Acceptable behaviour: the first racer reports `is_new=true`, the second reports `is_new=false`; both outcomes are correct descriptions of what each request observed. Documented in the error-handling section.
- **Risk**: A future second paper source (e.g. biorxiv) might want a SourceID format that overlaps with arxiv's (both use arxiv-style numeric IDs in some cases). — **Mitigation**: Composite key `(Source, SourceID)` already handles this by construction. No further action.

## References

- [GORM Indexes](https://gorm.io/docs/indexes.html) — composite unique index via struct tags.
- [GORM ErrDuplicatedKey](https://gorm.io/docs/error_handling.html) — driver-agnostic duplicate-key error surface.
- In-repo pattern precedent: `internal/infrastructure/persistence/source/` (full GORM repository with ToDomain/FromDomain).
- In-repo error envelope contract: `internal/domain/shared/errors.go` (`HTTPError`, `NewHTTPError`, `AsHTTPError`) and `internal/http/middleware/error_envelope.go`.
- Prior spec: `arxiv-fetcher` (defines `paper.Entry`, the `paper.UseCase` being renamed, and the arxiv fetch endpoint being modified).
