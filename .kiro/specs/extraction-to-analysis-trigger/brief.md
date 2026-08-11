# Brief: extraction-to-analysis-trigger

## Problem

Once `pdf-to-extraction-trigger` closes the download→extraction gap, extraction still stops short of analysis: the operator must manually call `POST /api/analyses` with a hand-carried `extraction_id` to get the LLM summary + thesis-angle flag. This is the last manual hop in an otherwise fully automated fetch→download→extract→analyze pipeline.

## Current State

- `document-extraction`'s worker (`application/extraction.Worker`) drains pending rows one at a time via `Process`, transitioning each to `done` or `failed`. Nothing observes that transition externally — the `Notifier`/`ChannelNotifier` wake-channel exists only to signal the worker's own pickup loop, not downstream consumers.
- `llm-analyzer`'s `analyzer.UseCase.Analyze` is deliberately synchronous — its design explicitly rules out "async or worker-driven analysis" as out-of-boundary for the analyzer itself. It reads `body_markdown` from the extraction row via `extraction.Repository.FindByID` and makes a real LLM call (`shared.LLMClient.Complete`), which is not instant.
- `POST /api/analyses` today requires the caller to already know the `extraction_id` — there is no path from "an extraction just finished" to "an analysis exists" without a manual request.

## Desired Outcome

The moment an extraction transitions to `done`, analysis is submitted automatically for it — no operator action, no hand-carried `extraction_id`. Applies unconditionally to every successful extraction, including re-extractions (a re-submitted paper overwriting a prior row re-triggers a fresh analysis, consistent with treating every success the same way). A `failed` extraction never triggers analysis — silent stop, visible via the existing extraction status endpoint and structured logs, same as today.

## Approach

- Extend `application/extraction`'s worker with a new injected port (name TBD in design, e.g. `AnalysisTrigger`) invoked right after a row transitions to `done`. Same architectural shape as `pdf-to-extraction-trigger`'s port, for consistency.
- Unlike that sibling spec, this call-site must **not** block the extraction worker's own single-threaded drain loop: `analyzer.UseCase.Analyze` is a real LLM round-trip (multi-second), and `document-extraction`'s worker already processes one row at a time — an inline call here would visibly slow down extraction throughput. The trigger adapter dispatches the downstream `Analyze` call asynchronously (a bounded/logged background call, not a blocking one) rather than reusing the fully-synchronous pattern from `pdf-to-extraction-trigger` verbatim. Exact mechanism (single goroutine per event vs. a small bounded pool) is a design-phase decision, not a roadmap-level call — but "unbounded goroutine-per-event with no backpressure" should be explicitly evaluated and either justified or rejected in the design doc, not silently defaulted into.
- The analyzer's own synchronous, non-worker nature is preserved exactly as `llm-analyzer` specified it — this spec adds a new *caller* of `analyzer.UseCase.Analyze`, it does not turn the analyzer itself into a worker.

## Scope

- **In**:
  - New port on `application/extraction` (or a package it depends on) invoked when a row reaches `done`.
  - Bootstrap wiring of a concrete adapter that calls `analyzer.UseCase.Analyze` off the extraction worker's own call path (async dispatch, per Approach above).
  - Re-extraction (row overwritten and re-processed to `done`) triggers a fresh analysis the same way a first-time success does.
- **Out**:
  - Any change to `analyzer.UseCase.Analyze`'s contract, `llm-analyzer`'s domain/persistence, or its synchronous-only design.
  - Any change to `document-extraction`'s domain/persistence beyond the new port call-site in its worker.
  - Turning the analyzer into a worker or adding retry/backoff to LLM calls — `llm-analyzer` already ruled this out for itself; this spec doesn't reopen that boundary.
  - Any new HTTP endpoint or status surface — the existing `GET /api/analyses/:extraction_id` already serves the auto-submitted analysis once it exists.
  - The download→extraction leg (separate, already-scoped spec: `pdf-to-extraction-trigger`).

## Boundary Candidates

- The new trigger port lives in the extraction orchestration layer (upstream), not inside `llm-analyzer` (downstream) — mirrors `pdf-to-extraction-trigger`'s split exactly, for the same reason: the downstream spec shouldn't need to know whether its use case was invoked by a human or a trigger.
- The async-dispatch mechanism (whatever it turns out to be in design) is this spec's own concern — it must not require `llm-analyzer` to change its synchronous contract to accommodate being called from a background goroutine instead of an HTTP handler.

## Out of Boundary

- Anything inside `internal/domain/analyzer`, `internal/application/analyzer`, or `internal/infrastructure/persistence/analyzer` — this spec is a caller of `analyzer.UseCase`, not a modifier of it.
- Anything inside `internal/domain/extraction`'s core state machine — only the worker's post-`done` call-site is touched.
- Retry, backoff, or dead-letter handling for a failed auto-triggered analysis call (matches `llm-analyzer`'s own "no retry" stance).
- The download→extraction handoff.

## Upstream / Downstream

- **Upstream**: `application/extraction.Worker` (the row reaching `done` inside `Process`), fed in turn by `pdf-to-extraction-trigger`'s auto-submitted rows (and any manually-submitted ones — trigger applies unconditionally).
- **Downstream**: `analyzer.UseCase.Analyze` (unchanged, called as-is).

## Existing Spec Touchpoints

- **Extends**: `document-extraction` (adds a call-site inside its worker, same additive spirit as `pdf-to-extraction-trigger`'s change to the pdfdownload worker).
- **Adjacent**: `llm-analyzer` (consumed, not modified), `pdf-to-extraction-trigger` (sibling; sequenced after it in the roadmap so the injected-port pattern is proven once, on the simpler synchronous case, before this spec's added async-dispatch complexity).

## Constraints

- Go 1.25 + Gin + GORM/SQLite per `tech.md`. Dependency rule per `structure.md`: the new port lives in `domain/`, implementation/wiring in `infrastructure/`/`bootstrap/`.
- Async dispatch must not silently swallow errors — a failed auto-triggered `Analyze` call needs the same structured-log visibility a failed extraction already gets.
- Single global behavior — no per-request opt-out of auto-triggering (per confirmed scope: applies unconditionally, including re-extractions).
