# Brief: arxiv-pdf-download

## Problem
After `/api/arxiv/fetch` returns paper entries (each carrying a `PDFURL`),
nothing materializes the PDF bytes. `pdf.Store.Ensure` exists and is wired
into `route.PDFConfig.Store`, but no caller invokes it for newly fetched
entries, so the local artifact cache stays empty until some downstream
consumer (e.g. extraction) lazy-triggers a download — which today does not
happen automatically. Users cannot work with the actual PDF content.

## Current State
- `internal/application/arxiv/usecase.go` fetches entries and persists them.
- `internal/domain/paper/model.go` carries `PDFURL` per entry.
- `internal/domain/pdf` defines `Store.Ensure(ctx, Key) (Locator, error)`.
- `internal/infrastructure/pdf/local/store.go` implements byte-fetch +
  atomic publish + cache gate. Idempotent under concurrent calls.
- No code path connects "arxiv fetch returned" → "Ensure each PDF".
- No client-facing transport exists for asynchronous per-entry status.

## Desired Outcome
- After `/api/arxiv/fetch` persists a batch, PDFs for every `IsNew` entry
  are fetched into the local store without blocking the HTTP response.
- The HTTP response immediately returns the entries plus a `job_id` and
  the `source_id`s scheduled for download.
- The client subscribes to a per-job SSE stream and receives one event per
  entry (success or failure with category) plus a terminal summary.
- A REST fallback exposes the same per-job result for clients that missed
  the stream or want to poll.
- A single failed download never aborts the batch: per-entry failures are
  isolated, logged, and surfaced as `failed` events.

## Approach
Inline trigger inside the arxiv use case behind a new application port
`pdfdownload.Scheduler`, fire-and-forget execution, in-memory `JobRegistry`
with replay buffer, SSE transport for client notifications. Concretely:

1. **New domain port** in `internal/domain/pdf/ports.go` (or a sibling
   `internal/application/pdfdownload/`) — a `Scheduler` interface that
   takes a slice of (Key, source_id) and returns a `JobID`. The arxiv
   use case depends on this port; the implementation lives in
   infrastructure/application and wraps `pdf.Store.Ensure`.
2. **Job registry** — in-memory `map[JobID]*Job` guarded by a mutex.
   Each `Job` holds: total count, per-entry results, ring buffer of
   pending events, list of subscriber channels, completion flag. TTL
   eviction (default 5 minutes after completion) so memory stays bounded.
3. **Worker** — one goroutine per scheduled job iterates entries,
   calls `Store.Ensure`, classifies result via existing `pdf.Category*`
   taxonomy, appends event to ring buffer, fans out to subscribers.
   Per-entry failure isolated: log + continue + `failed` event.
4. **Arxiv hookup** — `arxiv.UseCase.Fetch` calls `Scheduler.Schedule`
   for entries where `IsNew` is true after persistence completes.
   Returns `job_id` and the scheduled `source_id`s in its response.
   `Schedule` returns immediately; downloads run in the background.
5. **SSE endpoint** — `GET /api/arxiv/downloads/{job_id}/stream`. On
   connect: replay buffered events, then forward live events. Sends a
   terminal `download.summary` event and closes the stream when the
   job finishes. Honors client disconnect.
6. **Status endpoint** — `GET /api/arxiv/downloads/{job_id}` returns
   `{ job_id, total, succeeded, failed, entries: [...] }` for clients
   that missed the stream.
7. **Future-mode TODO** — comment in the arxiv use case noting that the
   download trigger may move out of `Fetch` into a separate user-confirmed
   call ("show list → user clicks OK → download"). Today it stays inline
   to keep the slice small.

Why this approach: keeps the domain dependency rule (arxiv depends on a
port, not on `pdf.Store`); reuses existing idempotent `Store.Ensure`
unchanged; SSE matches the one-way notification traffic without WebSocket
overhead; job IDs eliminate the SSE-subscription race; replay buffer plus
status endpoint covers reconnect and missed-event cases.

## Scope
- **In**:
  - `pdfdownload.Scheduler` port + implementation + job registry.
  - Background worker invoking `pdf.Store.Ensure` per entry.
  - Per-entry failure isolation using existing `pdf.Category*` taxonomy.
  - `arxiv.UseCase.Fetch` calling `Scheduler.Schedule` for `IsNew` entries.
  - `arxiv` HTTP response shape extended with `job_id` and
    `scheduled_for_download` list.
  - SSE endpoint `GET /api/arxiv/downloads/{job_id}/stream` with replay.
  - Status endpoint `GET /api/arxiv/downloads/{job_id}`.
  - In-memory job registry with TTL eviction.
  - `pdf.Key` construction from `paper.Entry` (source_type="arxiv",
    source_id=Entry.SourceID+Version, url=Entry.PDFURL).

- **Out**:
  - User-confirmation flow ("show list, click OK, then download") — only a
    TODO comment for now; design lives in a future spec.
  - Persisting download status across restarts (job registry is in-memory).
  - Retry / backoff policy beyond what `Store.Ensure` already does.
  - Re-downloading entries where `IsNew == false`.
  - Concurrency tuning (parallel downloads per job) — sequential first.
  - WebSocket transport.
  - Authentication / authorization on the SSE endpoint (inherits whatever
    middleware the existing arxiv routes use).
  - Frontend changes.

## Boundary Candidates
- `pdfdownload` application package owning `Scheduler`, `Job`, registry.
- `pdfdownload` HTTP controller owning SSE + status endpoints.
- Worker logic kept inside `pdfdownload` (not in `infrastructure/pdf/`):
  it's orchestration, not byte-fetch.
- `arxiv.UseCase.Fetch` only depends on the `Scheduler` interface.

## Out of Boundary
- The pdf-download port does NOT own arxiv-specific logic; it operates on
  generic `(pdf.Key, source_id)` pairs so future sources reuse it.
- The pdf-download port does NOT mutate `paper.Entry` or own paper state.
- `pdf.Store` keeps its current narrow contract — no progress callbacks,
  no event publication baked in. Events live one layer up in the worker.
- Persistence of download outcomes (e.g. an `artifact_status` table) is
  explicitly deferred.

## Upstream / Downstream
- **Upstream**:
  - `internal/application/arxiv` (calls `Scheduler.Schedule` after persist).
  - `internal/domain/paper.Entry` (carries `PDFURL`, `SourceID`, `Version`).
  - `internal/domain/pdf.Store` (used by the worker; unchanged).
  - `internal/domain/shared.Logger`, `shared.Clock` (registry TTL).
  - `internal/http/route` (registers new SSE/status routes).

- **Downstream**:
  - Future "user-confirmed download list" spec consumes the same Scheduler.
  - Extraction pipeline (`internal/application/extraction`) likely becomes
    a Scheduler subscriber later, so it can start work as soon as a PDF
    lands instead of polling the store.
  - Frontend SSE client (separate spec).

## Existing Spec Touchpoints
- **Extends**: none — no existing kiro specs in `.kiro/specs/`.
- **Adjacent**:
  - `internal/application/arxiv` — modified to call the new Scheduler.
  - `internal/infrastructure/pdf/local` — unchanged.
  - `internal/http/route/route.go` — new sub-bundle for the pdf-download
    config (Scheduler + JobReader for the status endpoint).

## Constraints
- Follow `backend/CLAUDE.md` dependency rule: arxiv use case depends on
  a port, never on the concrete worker or `pdf.Store`.
- All external-system calls go through interfaces; the worker depends on
  `pdf.Store`, not `httpclient` directly.
- `context.Context` first parameter on every method.
- `log/slog` only via `domain/shared.Logger`.
- Test standards: real-over-fake DB where DB is involved; doubles in
  `tests/mocks/`; sentence subtest names; AAA via blank lines.
- No production changes for test observability — use `t.Logf` in tests.
- Job registry must be safe under concurrent Schedule, Subscribe, and
  worker writes; mutex-guarded, document the lock discipline.
- SSE handler must honor client `ctx.Done()` and stop writing on
  disconnect to avoid goroutine leaks.
- Fire-and-forget worker must NOT use the request context (it outlives
  the request); use a background context derived at registry construction
  time, with explicit shutdown hook.
