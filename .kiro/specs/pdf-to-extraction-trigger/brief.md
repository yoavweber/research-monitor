# Brief: pdf-to-extraction-trigger

## Problem

The sole-user researcher runs `GET /api/arxiv/fetch`, which persists new papers and auto-downloads their PDFs (`arxiv-pdf-download`) — but the pipeline stops there. To get an extraction, the operator must manually call `POST /api/extractions` with a hand-supplied `pdf_path`, even though the PDF's on-disk location is already known to the system the moment the download succeeds. This is exactly the kind of manual ID/path hand-carrying the pipeline was supposed to eliminate.

## Current State

- `arxiv-pdf-download`'s `application/pdfdownload.Registry` runs a per-entry worker that fetches each PDF into `pdf.Store` and records an in-memory `EntryResult` (success/failure + category), fanned out over SSE and readable via `GET /api/arxiv/downloads/:job_id`.
- `document-extraction`'s `extraction.UseCase.Submit` accepts a `RequestPayload{SourceType, SourceID, PDFPath}` and enqueues a pending row; its own design explicitly disclaims owning "PDF download / staging" — the caller supplies `pdf_path`.
- Nothing today observes a pdfdownload entry's success and calls `extraction.UseCase.Submit` on its behalf. The pdfdownload registry's job state is in-memory only and evicted after a retention window (`PDF_DOWNLOAD_RETENTION`), so a polling bridge would risk missing entries — the trigger must be synchronous with the download's own completion.

## Desired Outcome

The moment a PDF download succeeds, extraction is submitted automatically with the correct `pdf_path` (the same canonical path `pdf.Store` wrote to) and `source_type`/`source_id` derived from the paper's identity — no operator action, no hand-carried path. Applies unconditionally to every successful download, regardless of whether the download job itself was triggered by the automated arxiv fetch flow or (future) any other caller of the pdfdownload registry.

## Approach

- Extend `application/pdfdownload`'s per-entry worker with a new injected port (name TBD in design, e.g. `ExtractionTrigger`) invoked synchronously right after a successful `pdf.Store.Ensure` call, before/alongside the existing `EntryResult` recording. Mirrors the existing precedent: `arxivUseCase.Fetch` already calls `Scheduler.Schedule` post-persist in the same in-process, synchronous style.
- The trigger's production adapter (bootstrap-wired) calls `extraction.UseCase.Submit(ctx, RequestPayload{...})` directly. `Submit` is a fast DB write (enqueue + wake signal) — no async dispatch needed for this leg, unlike the extraction→analysis leg where the downstream call is a slow LLM round-trip.
- Failed downloads (`EntryResult.Status != success`) never trigger extraction — silent stop, visible only via the existing download status/SSE surface and structured logs. No new failure-visibility surface.
- A re-download of the same paper (if that ever becomes possible) would re-trigger extraction the same way a fresh success does — `extraction.UseCase.Submit` already handles re-enqueue/overwrite idempotently.

## Scope

- **In**:
  - New port on `application/pdfdownload` (or a package it depends on) invoked on a successful per-entry download.
  - Bootstrap wiring of a concrete adapter that calls `extraction.UseCase.Submit`.
  - Deriving the correct `RequestPayload` (source type/id, and the exact `pdf_path` the store wrote to) from the completed `EntryResult`.
- **Out**:
  - Any change to `extraction.UseCase.Submit`'s contract, `document-extraction`'s domain/persistence, or its worker.
  - Any change to `pdf.Store`'s contract or the byte-fetch path.
  - The extraction→analysis leg (separate spec: `extraction-to-analysis-trigger`).
  - Any new HTTP endpoint or SSE event — the existing extraction status endpoint already surfaces the auto-submitted row.
  - Retry of a failed download or a failed `Submit` call.

## Boundary Candidates

- The new trigger port lives in the pdfdownload orchestration layer (upstream), not inside `document-extraction` (downstream) — `document-extraction`'s design explicitly disclaims "PDF download / staging," so it should have no awareness that its `Submit` calls might originate from an automated trigger vs. a manual API call.
- Port shape stays generic to "a completion happened, here's the artifact" — it should not leak pdfdownload-specific concepts (job IDs, SSE event types) into `extraction.RequestPayload`.

## Out of Boundary

- Anything inside `internal/domain/extraction`, `internal/application/extraction`, or `internal/infrastructure/persistence/extraction` — this spec is a caller of `extraction.UseCase`, not a modifier of it.
- Anything inside `internal/domain/pdf` or `internal/infrastructure/pdf/local` — `pdf.Store`'s contract is unchanged.
- The extraction→analysis handoff.
- Frontend rendering of any new pipeline state.

## Upstream / Downstream

- **Upstream**: `application/pdfdownload`'s per-entry worker (`worker.go`'s `runEntry`/`emitEntryCompleted`), the existing `pdf.Store`.
- **Downstream**: `extraction.UseCase.Submit` (unchanged, called as-is). `extraction-to-analysis-trigger` (a sibling spec) picks up from where this one's output — a submitted extraction row — leaves off.

## Existing Spec Touchpoints

- **Extends**: `arxiv-pdf-download` (adds a call-site inside its worker) in the same additive spirit that spec itself used when it added a `Scheduler.Schedule` call inside `application/arxiv`'s use case.
- **Adjacent**: `document-extraction` (consumed, not modified), `extraction-to-analysis-trigger` (sibling, sequenced after this one so the injected-port pattern is proven once before repeating it).

## Constraints

- Go 1.25 + Gin + GORM/SQLite per `tech.md`. Dependency rule per `structure.md`: the new port lives in `domain/` (whichever package owns it), implementation/wiring in `infrastructure/`/`bootstrap/`.
- Must not make `application/pdfdownload`'s worker block meaningfully longer per entry — `extraction.UseCase.Submit` is fast enough to call inline, but the design phase should confirm this against the worker's existing throughput assumptions.
- Single global behavior — no per-request opt-out of auto-triggering (per confirmed scope: applies unconditionally).
