# Brief: document-extraction

## Problem

**Who**: The researcher running the personal DeFi research monitor — sole user.

**Pain**: Papers fetched and persisted by the existing pipeline (`arxiv-fetcher` + `paper-persistence`) are metadata-only. The PDF bodies — which carry the math, methods, and arguments that determine whether a paper is thesis-relevant — are inaccessible to downstream LLM summarization and thesis-angle analysis. There is no way to convert a stored paper's PDF into structured, math-faithful markdown that an LLM consumer can reason over.

## Current State

- `paper.Entry` carries `PDFURL` and `AbsURL` (set by `arxiv-fetcher`) but the PDF body itself is never fetched, stored, or normalized.
- `product.md` lists `extract body (HTML / PDF)` as a pipeline stage; no implementation exists.
- `tech.md` previously listed `github.com/ledongthuc/pdf` as a planned PDF library — superseded by this spec, which uses MinerU for math fidelity. HTML extraction (`html-to-markdown`) remains a separate future spec.
- No extraction port, adapter, or persistence exists in the codebase.
- The system has no concept of an asynchronous job — every existing HTTP endpoint completes synchronously.

## Desired Outcome

- A new domain aggregate `extraction` with three ports:
  - `extraction.Extractor` — single-method port: takes a PDF path + source-type, returns a raw markdown body and (in the same call) the title heuristic / metadata that the underlying tool produces, or a typed extraction error.
  - `extraction.Repository` — persists extraction artifacts and tracks job lifecycle in one row per `(source_type, source_id)`.
  - `extraction.UseCase` — orchestrates: enqueue / re-enqueue, drive a job through the state machine, surface status.
- A SQLite-backed repository at `infrastructure/persistence/extraction/`, single `extractions` table carrying both job state and artifact body inline (separate from the `papers` metadata table). The full request input (`source_type`, `source_id`, `pdf_path`) is persisted as a JSON blob (`request_payload`) on the row, so the worker reads every execution input directly from the DB and never depends on in-flight memory state from the request handler.
- A subprocess-based MinerU adapter at `infrastructure/extraction/mineru/`, swappable behind `Extractor` so a future Mathpix or `mineru-api`-over-HTTP impl drops in without touching consumers.
- A normalizer (pure domain code) that strips the references / bibliography section, drops table / image / figure-caption blocks, normalizes math delimiters to `$...$` (inline) and `$$...$$` (display), computes whitespace-split word count, and chooses a title (first `#` heading, fallback to source filename).
- A single in-process worker goroutine that pulls the oldest `pending` row from the repository, calls `Extractor` + normalizer, writes back. Started and stopped from bootstrap.
- A buffered, signal-only notification channel (`chan struct{}`, capacity from `extraction.signal_buffer`, default `10`) owned by the worker. `POST /api/extractions` sends exactly one signal after the row is committed; the channel carries no job data. On startup — after recovery and after the expiry / pending sweep are reconciled — the worker queries for every existing `pending` row and self-signals once per row before entering its main loop, so any work that arrived (or stalled) while the process was down is drained without operator intervention. A non-blocking send is used so a full buffer drops the signal harmlessly: the worker re-checks the DB after every wake and will discover unsent work on the next signal anyway.
- HTTP endpoints under the authenticated `/api` group:
  - `POST /api/extractions` — accepts `{ source_type, source_id, pdf_path }`, creates or overwrites the row keyed by `(source_type, source_id)`, persists the input as the `request_payload` JSON blob, enqueues the job, sends one wake signal on the worker channel, returns `202 Accepted` with `{ id, status: "pending" }`.
  - `GET /api/extractions/:id` — returns current job status and, when status is `done`, the full artifact (`title`, `body_markdown`, `metadata { source_url, fetch_date, content_type, word_count }`).
- Configurable maximum word count (`extraction.max_words`, default `50000`); when a successful extraction exceeds it, the job transitions to `failed` with a typed `ErrTooLarge` so the caller can distinguish from other failure modes via the structured error payload.
- Configurable per-job expiry checked **at worker pickup** (`extraction.job_expiry`, default `1h`). The worker, immediately after dequeuing a row and before invoking the extractor, compares `now()` against `created_at + job_expiry`; if exceeded, the row transitions to `failed: expired` regardless of whether its prior status was `pending` or `running` (the latter applying when an in-flight job is reconciled at restart). No background goroutine, no ticker, no separate sweep — expiry is a pickup-time check only. Caller may re-`POST` to retry.
- On process restart, any rows still in `running` status are atomically transitioned to `failed` with reason `process_restart` before the worker starts — the worker that owned them is gone and the PDF is the durable source of truth, so re-`POST` from the caller is the recovery path.

## Approach

Approach **A** from discovery, with **async** job semantics end to end.

Rationale:
- MinerU 3.x already auto-spawns a persistent local `mineru-api` server under its CLI and reuses it across invocations. The cold-start cost that distinguished a per-call subprocess from a long-lived sidecar largely dissolves in practice — they converge. Starting from the simpler subprocess shape keeps the spec self-contained (no second binary in `task run`, no port to manage), and the `Extractor` interface lets us migrate to a Go-managed `mineru-api` HTTP client later with zero domain churn.
- Async lifecycle is mandatory because MinerU latency is on the order of 30s–several minutes per paper. A sync HTTP request would either time out or block a connection for the full extraction. Async also persists failures naturally — every job's terminal state is queryable via `GET /api/extractions/:id`.

## Scope

- **In**:
  - Domain ports: `extraction.Extractor`, `extraction.Repository`, `extraction.UseCase`. Typed sentinels following the `*shared.HTTPError` pattern from `source` / `paper`: `extraction.ErrScannedPDF`, `extraction.ErrTooLarge`, `extraction.ErrExpired`, `extraction.ErrParseFailed`, `extraction.ErrExtractorFailure`, `extraction.ErrNotFound`.
  - Domain value objects: `extraction.Artifact` (id, source_type, source_id, title, body_markdown, metadata, word_count, timestamps), `extraction.RequestPayload` (`source_type`, `source_id`, `pdf_path`) — JSON-marshaled into the persistence row, `extraction.JobStatus` (`pending` | `running` | `done` | `failed`), `extraction.FailureReason` enum (includes `process_restart`, `expired`, `too_large`, `scanned_pdf`, `parse_failed`, `extractor_failure`).
  - Pure-domain normalizer (`domain/extraction/normalize.go`): heading-based reference / bibliography / works-cited stripping (regex `^(references|bibliography|works cited)$` case-insensitive on a heading line, drop everything after), table / image / figure-caption block skipping, math delimiter normalization to the brief's `$...$` / `$$...$$` contract, whitespace-token word count, title extraction.
  - SQLite persistence at `infrastructure/persistence/extraction/`: single `extractions` table, inline `body_markdown TEXT`, JSON-encoded `request_payload TEXT` column holding the full request input, unique index on `(source_type, source_id)`, composite index on `(status, created_at)` for cheap worker pickup ordering. Persistence model + `ToDomain` / `FromDomain` mirrors the `paper` aggregate. JSON marshal / unmarshal of `request_payload` follows the `Authors` / `Categories` precedent in `infrastructure/persistence/paper/model.go`.
  - Auto-migration of the new table hooked into the existing `persistence.AutoMigrate(db)` call.
  - MinerU adapter at `infrastructure/extraction/mineru/`: `exec.CommandContext(ctx, "mineru", "-p", pdfPath, "-o", tmpOutDir, ...)`, reads the produced `.md` from the bundle directory, ignores image / JSON sidecars, classifies CLI exit codes / stderr patterns into typed domain errors, cleans up the temp dir. Binary path and per-call timeout are configurable.
  - In-process worker at `application/extraction/`: single goroutine driven by a buffered `chan struct{}` (capacity from `extraction.signal_buffer`, default `10`). On each wake the worker dequeues the oldest `pending` row, reads its `request_payload` JSON to obtain execution inputs, evaluates the pickup-time expiry check (compare `clock.Now()` against `created_at + extraction.job_expiry`, default `1h`), and either transitions the row to `failed: expired` or proceeds: `pending → running → done | failed`. After every wake the worker drains by re-checking the DB until no pending rows remain, so a coalesced wake (full buffer) cannot leave work stranded. Lifecycle (start / graceful stop) wired in bootstrap.
  - Worker startup sequence: (a) bootstrap invokes the repository's recovery method to flip any `running` rows to `failed: process_restart`; (b) bootstrap queries for all currently `pending` rows and self-signals the worker channel once per row; (c) only then is the worker goroutine launched. This guarantees that work pending at restart is picked up without operator intervention and without a polling ticker.
  - `POST /api/extractions` performs a non-blocking send on the worker channel after the row is committed; a full buffer drops the signal, which is safe — the worker re-checks the DB after every successful drain and will pick up the row on its next wake.
  - Two HTTP endpoints under the authenticated `/api` group (existing `APIToken` middleware):
    - `POST /api/extractions` — enqueue / re-extract.
    - `GET /api/extractions/:id` — status + artifact when done.
  - Re-extraction: if a row already exists for `(source_type, source_id)`, overwrite it (reset to `pending`, refresh `created_at` so the new request gets a full `job_expiry` window, replace `request_payload` with the new request body, clear prior body / error fields, keep the row id) and emit a structured `extraction.reextract` log line capturing the prior status and reason. PDF path always taken from the request — caller is responsible for the file being on disk and readable.
  - Configurable thresholds via `viper`-bound config block (`extraction.*`): `max_words` (default `50000`, surfaced into the normalizer / use case so the failure mode is checked deterministically after extraction), `signal_buffer` (default `10`, capacity of the worker wake channel), `job_expiry` (default `1h`, used by the pickup-time expiry check), plus `mineru_path` and `mineru_timeout` for the adapter.
  - Controller-owned wire DTOs (`ExtractionStatusResponse`, `ExtractionArtifactResponse`) in `internal/http/controller/extraction/`, same style as the arxiv / paper controllers.
  - Bootstrap wiring: build extractor → repository → use case → worker (channel + deps, goroutine not yet launched) → controller → register routes → run on-startup recovery (`running → failed: process_restart`) → enumerate `pending` rows and self-signal once per row → start worker goroutine.
  - Unit tests: normalizer (heading strip, math normalization both delimiter directions, table / image skip, word count, title fallback), repository (CRUD + unique-index dedupe + recovery flip + `request_payload` round-trip), use case (enqueue creates row, re-enqueue overwrites + refreshes `created_at` + logs, status transitions, typed-error pass-through), worker (signal-driven dequeue, drain-loop after wake, pickup-time expiry against an injected `shared.Clock`, non-blocking send on a full channel), startup sequencing (recovery flip + per-pending self-signal before goroutine launch). MinerU adapter unit-tested with an injectable command runner. Real CLI exercised by an integration test gated behind a `mineru` build tag so default `task test` stays hermetic.
  - Integration tests via existing `SetupTestEnv` for both endpoints, including 401, 404, validation errors, and a full happy-path with a fake `Extractor` driving `pending → done`.

- **Out**:
  - PDF acquisition / download. Caller supplies `pdf_path`; a future spec (or direct edit) wires `paper.Entry.PDFURL` → local download.
  - HTML extraction (deferred to a separate spec per user's brief).
  - OCR / scanned-PDF handling — the adapter classifies that case as `ErrScannedPDF` and stops there.
  - Table extraction, image extraction, figure-caption preservation, equation rendering, multi-column layout fixes beyond what MinerU emits natively.
  - Batch / fan-out extraction ("extract every unprocessed paper from yesterday's fetch") — clean follow-up spec layered on the same `Extractor` port.
  - LLM summarization, thesis-angle flagging, triage. This spec produces the input that those stages consume; it does not run them.
  - Webhooks or push notifications on job completion. Status is poll-only via `GET /api/extractions/:id`.
  - Automatic retry of failed jobs. Caller decides to re-`POST`.
  - Multi-worker concurrency. v1 ships with exactly one worker goroutine.
  - Cancellation endpoint (`DELETE /api/extractions/:id`) for in-flight jobs.
  - Background expiry sweeper / ticker. Expiry is checked **only** at worker pickup time; rows that exceed `job_expiry` while sitting `pending` and never get picked up will simply transition to `failed: expired` whenever the worker eventually does dequeue them. No periodic goroutine sweeps the DB.
  - Persistence of the worker wake channel across restarts. The channel is in-memory; restart-time recovery (the per-pending self-signal step) is what reconciles state.
  - Auto-purge / housekeeping for old completed rows.
  - Auth changes — inherits the existing `APIToken` middleware unchanged.

## Boundary Candidates

- **Domain ports + typed errors** (`extraction.Extractor`, `extraction.Repository`, `extraction.UseCase`, sentinels) — the contract surface consumed upward.
- **Pure normalizer** (`domain/extraction/normalize.go`) — deterministic, side-effect free, fully unit-testable in isolation; this is where math fidelity is enforced.
- **MinerU adapter** (`infrastructure/extraction/mineru/`) — subprocess invocation, output-bundle parsing, error classification (CLI exit code / stderr → typed domain error), temp-dir lifecycle.
- **Persistence adapter** (`infrastructure/persistence/extraction/`) — schema, indexes, model + `ToDomain` / `FromDomain`, recovery method.
- **Worker** (`application/extraction/worker.go`) — owns the buffered signal channel, the drain loop, the pickup-time expiry check, and the job state machine. Reads execution inputs only from the row's `request_payload`. Owns nothing else.
- **HTTP layer** (`internal/http/controller/extraction/`) — handlers + DTOs + `ExtractionRouter` wired into `route.Setup`.
- **Bootstrap wiring** — auto-migrate, build deps, register routes, run on-startup recovery, start worker, plumb shutdown.

## Out of Boundary

- The product-pipeline triage / summary stages — this spec stops at "normalized markdown is queryable".
- `paper.Entry` write-side semantics — this spec does not modify or extend `paper-persistence`; the link between an `extraction` row and a `papers` row is the `(source_type, source_id)` convention only, not a foreign key.
- `arxiv-fetcher` requirement 1.4 ("shall not write fetched entries to any datastore") and the `paper-persistence` boundary remain unchanged.
- Any non-PDF source. `source_type` is on the wire today only because the storage shape is generic and to keep `paper` from being baked into the domain; v1 only accepts `paper`.
- Multi-tenant or multi-user behavior — single-user model from `product.md` continues.

## Upstream / Downstream

- **Upstream**:
  - `paper.Entry.PDFURL` / `paper.Entry.SourceID` — data identifying which paper a request maps to. The caller of `POST /api/extractions` reads these and supplies the request body; this spec does not import `paper`.
  - `shared.HTTPError`, `shared.Logger`, `shared.Clock`.
  - `persistence.AutoMigrate(db)` and the shared `*gorm.DB` from bootstrap.
  - The `APIToken` middleware on `/api`.
  - MinerU CLI on the host (`mineru`, Python ≥3.10 ≤3.13, macOS 14.0+ for Apple Silicon support, ≥16GB RAM, model weights downloaded — operator-supplied prerequisite). Documented in this spec, not installed by it.
  - Configuration: a new `Extraction` block in `bootstrap/env.go` exposing `max_words` (default `50000`), `signal_buffer` (default `10`), `job_expiry` (default `1h`, parsed as `time.Duration`), `mineru_path`, and `mineru_timeout`.
- **Downstream**:
  - A future LLM-summary / thesis-angle spec consumes `extraction.Artifact.body_markdown` and `title` to drive Anthropic API calls.
  - A future batch-extraction spec layers a fan-out trigger on top of the same `extraction.UseCase`.
  - A future "fetch + persist + extract" pipeline spec wires `arxiv-fetcher` output → PDF download → `POST /api/extractions`.
  - A future frontend reads `GET /api/extractions/:id` to surface extraction status alongside paper listings.

## Existing Spec Touchpoints

- **Extends**: none. New aggregate, parallel to `paper` and `source`.
- **Adjacent**:
  - `paper-persistence` — provides the `(source_type, source_id)` keys this spec uses to identify which paper an extraction belongs to. No schema or port is shared; the relationship is by convention.
  - `arxiv-fetcher` — produces the `PDFURL` upstream of any caller; touched only via `paper.Entry`.
  - `source` aggregate — pattern reference for `Repository` + `ToDomain` / `FromDomain` + auto-migration wiring layout. Mirror it.

## Constraints

- Go 1.25, GORM v2, SQLite (existing stack per `tech.md`). DB-agnostic at the port level — Postgres swap remains trivial.
- Dependency rule from `structure.md`: `domain/extraction/` may not import `infrastructure/`. All conversion stays on the persistence side via `ToDomain()` / `FromDomain()`.
- Naming per `structure.md`: ports `Extractor`, `Repository`, `UseCase` (callsite `extraction.Extractor`, etc.); implementing structs unexported; constructors `NewMineruExtractor(...)`, `NewRepository(db)`, `NewUseCase(...)` return interfaces.
- `context.Context` is the first parameter of every port method, including `Extractor.Extract` — so subprocess cancellation propagates a client disconnect / shutdown signal into MinerU.
- `log/slog` only, via the `shared.Logger` port. Re-extraction events are explicit structured log entries.
- Hand-written fakes under `tests/mocks/`; no mock-generation tools.
- Math delimiter format from MinerU's CLI must be sample-verified during the **design** phase against a real DeFi paper before the normalizer is locked. Cannot be assumed without observation.
- MinerU output is a bundle directory: read only the `.md`, ignore image / JSON sidecars. The bundle directory is created under `os.TempDir()` per call and cleaned up after the markdown has been read into memory.
- License: MinerU "MinerU Open Source License" (Apache 2.0 + commercial-scale clauses) is fine for personal, non-revenue use. No GPL contamination as of MinerU 3.0.
- Restart-recovery is best-effort: a job in `running` at crash time becomes `failed: process_restart`; the caller may re-`POST` to retry. PDF on disk is the durable source of truth.
- Worker notification channel carries no payload — only a wake signal. The DB row is the single source of truth for execution inputs (`request_payload` JSON). Sends from the request handler are non-blocking; a full buffer is treated as a benign drop because the worker re-checks the DB on every wake.
- Expiry is a pickup-time predicate, not a scheduled sweep. The cost of a job sitting longer than `job_expiry` while `pending` is bounded by however long it takes the worker to reach that row in oldest-first order; no goroutine other than the worker reads the `extractions` table for liveness purposes.
- `extractions` rows are not auto-purged. Out of scope; future housekeeping can prune by `completed_at`.
