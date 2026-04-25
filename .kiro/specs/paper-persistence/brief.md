# Brief: paper-persistence

## Problem

**Who**: The researcher running the monitor — sole user of the backend.

**Pain**: Papers fetched via `/api/arxiv/fetch` are ephemeral. Every call is a fresh snapshot; there is no record of what has been seen, no way to review an accumulated set of candidates over time, and no foundation that later pipeline stages (dedupe-across-fetches, triage, LLM summary, frontend feed) can read from. Each fetch's result vanishes the moment the HTTP response closes.

## Current State

- `paper.Entry` exists as an immutable value object in `internal/domain/paper/` (produced by the `arxiv-fetcher` spec).
- `/api/arxiv/fetch` returns entries in the HTTP response only; requirement 1.4 of that spec forbids it from writing to any datastore.
- `internal/infrastructure/persistence/source/` is the established precedent for a GORM + SQLite repository (separate persistence model, `ToDomain` / `FromDomain` conversion, unexported struct, interface-returning constructor, auto-migration wired in bootstrap).
- No persistence exists for `paper.Entry`. `arxiv-fetcher`'s design explicitly lists persistence as out-of-boundary and names it as a revalidation trigger — opening this spec is that revalidation.

## Desired Outcome

- A `paper.Repository` domain port (new, in `internal/domain/paper/ports.go`) with three methods:
  - `Save(ctx, entry)` — idempotent on `SourceID`; first-seen wins, later occurrences are silently skipped (caller can learn whether insert happened via a return value such as `(wasNew bool, err error)` so stored-vs-skipped counts are observable).
  - `FindBySourceID(ctx, sourceID)` — returns `*paper.Entry` or a typed not-found error.
  - `List(ctx)` — returns all persisted entries, newest-first by `SubmittedAt`.
- A SQLite-backed implementation at `internal/infrastructure/persistence/paper/` that mirrors the `source` pattern exactly (persistence-layer model struct, `ToDomain` / `FromDomain`, unexported `repository`, `NewRepository(db)` returning the interface).
- Schema: a unique index on the `source_id` column enforces the dedupe rule at the database level so races degrade to clean errors, not silently duplicated rows.
- Auto-migration of the new table hooked into the existing `persistence.AutoMigrate(db)` call.
- Two read-only HTTP endpoints under the authenticated `/api` group:
  - `GET /api/papers` — list all persisted entries (newest-first).
  - `GET /api/papers/:source_id` — fetch one by source ID, or 404.
- Unit tests for the repository against a temp SQLite database; integration tests for the two endpoints through the existing `SetupTestEnv` harness.
- **No write-triggering HTTP endpoint** (no `POST /api/papers`). Writes are reachable only through the Go API.
- **No wiring to `/api/arxiv/fetch`**. Whether a fetch also persists is left for a follow-up, keeping arxiv-fetcher's requirement 1.4 intact for now.

## Approach

Approach C from discovery: storage layer + query surface only; ingestion integration deferred. Dedupe rule (i): dedupe on `SourceID` alone, first-seen wins. Follows the `source` aggregate's layout exactly so reviewers can map one-to-one between the two aggregates.

## Scope

- **In**:
  - `paper.Repository` interface (`Save`, `FindBySourceID`, `List`).
  - A new domain sentinel `paper.ErrNotFound` for the "no such paper" case, following the `source.ErrNotFound` pattern (`*shared.HTTPError` with code 404).
  - SQLite GORM implementation at `infrastructure/persistence/paper/` with a unique index on `source_id`.
  - Auto-migration of the new table at startup.
  - A query-oriented use case (`paper.QueryUseCase` or equivalent) that the HTTP layer depends on — keeps the controller thin and testable without poking at GORM directly.
  - `GET /api/papers` and `GET /api/papers/:source_id` under the authenticated `/api` group (inherits the existing `APIToken` middleware).
  - Controller-owned wire DTOs (`PaperResponse`, `PapersListResponse`) in `internal/http/controller/paper/`, same style as the arxiv controller.
  - Bootstrap wiring: construct repo → use case → controller → route.
  - Unit tests (repo + use case) and integration tests (both endpoints, including 401 and 404).

- **Out**:
  - Any write-triggering HTTP endpoint.
  - Auto-persist on `/api/arxiv/fetch` — deferred to a later small spec or a direct-implementation edit once this lands.
  - Version handling beyond "first-seen wins" (no upsert, no composite `(SourceID, Version)` key, no UpdatedAt-based replacement).
  - Delete / update / housekeeping endpoints.
  - PDF fetching, HTML extraction, LLM summarisation, triage, frontend consumption.
  - Multi-source discrimination (no `Source` column, no `(source_type, source_id)` composite key) — only the arxiv source exists today; `SourceID` uniqueness across all papers holds.
  - Pagination on `GET /api/papers` — acceptable while the table is small; can be added later without breaking the contract.
  - Rate-limiting, caching, or observability beyond the existing middleware and `shared.Logger` outcome logging.

## Boundary Candidates

- **Domain port + typed errors** (`paper.Repository` + `paper.ErrNotFound`) — the contract consumed upward.
- **Persistence adapter** (`infrastructure/persistence/paper/` — schema, model, `ToDomain` / `FromDomain`, dedupe via unique index).
- **Query use case** (`application/paper/` — the read-side orchestrator the controller consumes; a separate concern from the write-oriented repository).
- **HTTP query layer** (`http/controller/paper/` — handler + response DTOs + `PaperRouter` wired into `route.Setup`).
- **Bootstrap wiring** (builds repo → use case → controller → registers route).

## Out of Boundary

- `/api/arxiv/fetch` semantics stay unchanged — no automatic persistence on fetch.
- Requirement 1.4 of `arxiv-fetcher` ("shall not write fetched entries to any datastore") is not repealed by this spec.
- Any multi-source identity handling — assumes `SourceID` alone is globally unique for the foreseeable future.
- No scheduler, cron, or background loop.
- No version-upgrade semantics for papers that arxiv revises post-ingest.

## Upstream / Downstream

- **Upstream**:
  - `paper.Entry` (domain model, from `arxiv-fetcher`).
  - `shared.HTTPError`, `shared.Logger`, `shared.Clock`.
  - The existing `persistence.AutoMigrate(db)` hook and the shared `*gorm.DB` wired in bootstrap.
  - The `APIToken` middleware mounted at the `/api` group.

- **Downstream**:
  - A follow-up spec (or direct-implementation edit) that wires `arxiv-fetcher`'s fetch output into `paper.Repository.Save`, providing the end-to-end "fetch + persist" flow the personal-tool user eventually wants.
  - Any future triage / dedupe-across-fetches / summarisation stage reads from this repository.
  - A future frontend feed consumes `GET /api/papers`.

## Existing Spec Touchpoints

- **Extends**: none. This is a new aggregate layer, parallel to `source`.
- **Adjacent**:
  - `arxiv-fetcher` — produces the `paper.Entry` values this spec stores. This spec must not change that spec's contract; any integration between the two lives in a future spec.
  - `source` aggregate — the authoritative precedent for the Repository pattern, `ToDomain` / `FromDomain` split, and auto-migration wiring. Mirror it.

## Constraints

- Go 1.25, GORM v2, SQLite (existing stack per `tech.md`).
- Dependency rule from `structure.md` §2: `domain/paper/` must not import `infrastructure/persistence/paper/`. All conversion stays on the persistence side via `ToDomain()` / `FromDomain()`.
- Naming per `structure.md` §7: interface is `Repository` (callsite form `paper.Repository`), implementing struct is unexported, constructor `NewRepository(db)` returns the interface.
- `context.Context` is the first parameter of every use-case and repository method (steering §10).
- `log/slog` via the `shared.Logger` port only (steering §9).
- Tests: unit tests against a temp SQLite file (see `infrastructure/persistence/source/repo_test.go` for the pattern); integration tests via the existing `SetupTestEnv` harness, which already provides a temp DB and the authenticated `/api` group.
- The unique index on `source_id` must be declared in the persistence model struct so GORM creates it during auto-migration (no hand-written SQL migrations in the existing codebase).
