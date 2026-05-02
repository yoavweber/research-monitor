# Brief: llm-analyzer

## Problem

The pipeline (`fetch → dedupe → triage → extract → LLM summarise + thesis-angle flag → persist`, per [product.md](../../steering/product.md)) currently stops at extracted markdown. The user has no way to turn a 5k-word paper into something scannable from the research feed, and no signal for which papers are thesis-angle candidates — the core product promise.

## Current State

- `document-extraction` writes normalized `body_markdown` onto the `extractions` row, keyed by extraction `id` (UUID, primary key) with a unique composite on (`source_type`, `source_id`). See [persistence/extraction/model.go:23-28](../../../internal/infrastructure/persistence/extraction/model.go#L23-L28).
- `extraction.Repository.FindByID(ctx, id)` is already exposed at [domain/extraction/ports.go:18](../../../internal/domain/extraction/ports.go#L18).
- `domain/shared.LLMClient` port is already declared at [domain/shared/ports.go:37](../../../internal/domain/shared/ports.go#L37) but has zero implementations.
- `paper-persistence` is **not** in the analyzer's read path — domain `paper.Entry` exposes no UUID and the analyzer reads body markdown directly off the extraction row.
- No analyzer use case, no analysis storage, no analyzer HTTP surface exists.

## Desired Outcome

- `POST /analyses` with `{"extraction_id": "<uuid>"}` runs **synchronously**, executes three LLM calls (short summary, long summary, thesis-angle classification), persists the result to a new `analyses` table keyed by `extraction_id`, and returns the analysis JSON.
  - Response shape: `{ extraction_id, short_summary, long_summary, thesis_angle_flag (bool), thesis_angle_rationale, model, prompt_version, created_at, updated_at }`.
  - Status codes: `200 OK` on success (returns the freshly persisted row, whether new or overwritten), `400` malformed body, `404` extraction not found, `409` extraction not in `done` status (no `body_markdown` to analyze), `502` LLM upstream failure or repeated malformed JSON, `500` storage failure.
- Re-posting the same `extraction_id` re-runs and **overwrites** the prior row; `created_at` is preserved, `updated_at` advances.
- `GET /analyses/:extraction_id` returns the stored analysis or `404`.
- A fake `LLMClient` is wired in by default so the full path is exercisable end-to-end without an API key. The real provider adapter is a follow-up.

## Approach

**(a) Generic `shared.LLMClient` + analyzer use case that owns the prompts.** The analyzer use case calls `LLMClient.Complete` three times (short prompt, long prompt, thesis-flag prompt), persists the structured result via an analyzer Repository, and returns it. Prompts and prompt versions live inside `application/analyzer/` so the LLM port stays reusable for future LLM features (triage, classification).

Key seams:
- `domain/analyzer/`: `Analysis` model, `Repository`, `UseCase` ports, sentinel errors (`ErrExtractionNotFound`, `ErrExtractionNotReady`, `ErrLLMUpstream`, `ErrAnalyzerMalformedResponse`, `ErrAnalysisNotFound`, `ErrCatalogueUnavailable`).
- `application/analyzer/`: orchestrator that loads the extraction by `id`, validates `Status == done`, runs three `LLMClient.Complete` calls, parses the thesis-flag JSON envelope, persists, returns. Owns the prompts and prompt versions.
- `infrastructure/persistence/analyzer/`: GORM model + `Repository` impl backed by a new `analyses` table whose primary key is `extraction_id` (FK to `extractions.id`). Upsert path: `ON CONFLICT(extraction_id) DO UPDATE` rewrites `short_summary`, `long_summary`, `thesis_angle_flag`, `thesis_angle_rationale`, `model`, `prompt_version`, `updated_at`; `created_at` is preserved.
- `infrastructure/llm/fake/`: deterministic stub. Output is keyed by `LLMRequest.PromptVersion` (so each prompt type returns its own canned text, regardless of input markdown). The thesis-flag fake returns a valid JSON envelope so the parser path is exercised end-to-end.
- `interface/http/controller/analyzer/`: `POST /analyses`, `GET /analyses/:extraction_id`.

### LLM response contract

- **Short summary** and **long summary** calls: free-text completions. The use case stores the response text verbatim (whitespace-trimmed).
- **Thesis-angle** call: prompt instructs the model to return a strict JSON envelope `{"flag": <bool>, "rationale": "<string>"}` and nothing else. The use case parses with `encoding/json`, validates both fields are present and well-typed.
- **Parse failure / invalid envelope**: `ErrAnalyzerMalformedResponse` (HTTP 502). The request fails atomically and no analysis row is written. No retry — the real provider adapter can introduce retry/repair logic later if needed.
- **LLM transport failure** (`LLMClient.Complete` returns error): wrapped as `ErrLLMUpstream` (HTTP 502). No retry at this layer in this slice — retry/backoff is a future concern when the real adapter lands.

## Scope

- **In**:
  - New `domain/analyzer` package: `Analysis` value type (`extraction_id`, `short_summary`, `long_summary`, `thesis_angle_flag`, `thesis_angle_rationale`, `model`, `prompt_version`, `created_at`, `updated_at`), `UseCase`, `Repository`, sentinel errors.
  - New `application/analyzer` use case, synchronous, three LLM calls (short, long, thesis-angle), JSON-envelope parsing for the thesis call, fail-fast on parse failure.
  - New `infrastructure/persistence/analyzer` GORM model + repository + auto-migration. Single row per `extraction_id`; re-run overwrites with `created_at` preserved.
  - New `infrastructure/llm/fake` adapter implementing `shared.LLMClient`, deterministic by `PromptVersion`, returns valid JSON envelope for the thesis-flag prompt.
  - HTTP: `POST /analyses` (body: `{"extraction_id": "<uuid>"}`) and `GET /analyses/:extraction_id`. Status-code mapping per Desired Outcome.
  - Bootstrap wiring of the fake `LLMClient` as default.
  - `extraction_id` resolution path: `extraction.Repository.FindByID` + status check (`Status == done`); surface `ErrExtractionNotFound` / `ErrExtractionNotReady` distinctly.
- **Out**:
  - Real LLM provider adapter (Anthropic). Deferred to a follow-up spec/task once the analyzer skeleton is green.
  - Async / queued analysis (everything is synchronous in this slice).
  - Analysis history / versioning. Re-run overwrites the existing row.
  - Re-running on prompt-version change automatically (manual re-POST only).
  - Triage / news-vs-paper classification. Thesis-angle flag is scoped to "is this a thesis-angle candidate", nothing broader.
  - Listing / filtering endpoints (`GET /analyses` index). Only by-id GET in this slice.
  - Auth changes — uses the existing `X-API-Token` header.
  - Reading anything from `paper.Repository`. The analyzer never touches papers.
  - LLM retry/backoff of any kind. Transport errors and JSON-parse failures both fail the request immediately with 502. The real provider adapter owns retry/repair if needed later.

## Boundary Candidates

- `domain/analyzer.UseCase` vs `application/analyzer` orchestration: domain port stays thin (`Analyze(ctx, extractionID)`, `Get(ctx, extractionID)`); orchestration logic, prompt strings, and JSON-envelope parsing live in application.
- Analysis storage — own table, own repository, own GORM model. Does not extend `extractions` or `papers`.
- HTTP controller — own package under `interface/http/controller/analyzer`, mirroring the existing per-feature controller layout.
- Fake LLM adapter — its own package so the real adapter slots in without touching the analyzer use case.

## Out of Boundary

- Anthropic SDK integration (real `LLMClient` impl).
- Background workers, queues, retry policy for analyses (synchronous only).
- Cost / token budgeting and rate limiting.
- Multi-model routing (e.g., Haiku for short, Sonnet for long). The use case picks one model per call from config; routing is a follow-up if needed.
- Frontend rendering of summaries.
- Cross-extraction analytics, aggregations, or feed-level views.

## Upstream / Downstream

- **Upstream**:
  - `document-extraction` — must have produced a `done` row with `body_markdown` for the requested `extraction_id`. The analyzer fails with `ErrExtractionNotReady` (HTTP 409) if status is not `done`, and `ErrExtractionNotFound` (HTTP 404) if the id doesn't exist.
  - `domain/shared.LLMClient` — already declared; this spec ships the first implementation (a fake) and the first consumer.
- **Downstream**:
  - Real LLM provider adapter (Anthropic) — drop-in replacement for the fake.
  - Future thesis-feed UI consumes `GET /analyses/:extraction_id` (joined client-side with paper metadata via the shared `(source_type, source_id)` composite when needed).
  - Future triage / classification specs may reuse the same `shared.LLMClient` port and the prompt-versioning pattern established here.

## Existing Spec Touchpoints

- **Extends**: none. New spec, new boundary.
- **Adjacent**:
  - `document-extraction` (read-only consumer of `extraction.Repository.FindByID`).
  - `paper-persistence` (untouched — analyzer does not read papers).

## Constraints

- Go 1.25, Gin, GORM/SQLite per [tech.md](../../steering/tech.md). New `analyses` table via GORM auto-migration in bootstrap.
- Dependency rule per [structure.md](../../steering/structure.md): `domain/analyzer` imports only stdlib + `domain/shared`. Application imports domain. Infrastructure imports domain. No cross-imports between `domain/analyzer` and `domain/extraction` — composition happens in `application/analyzer`.
- Use existing `domain/shared.LLMClient` port; do **not** define a new analyzer-specific LLM port.
- Tests follow [testing.md](../../steering/testing.md) standards: real-over-fake DB for repository tests, hand-written fakes under `tests/mocks/`, sentence subtests, AAA via blank lines.
- Synchronous endpoint must complete within Gin's default request timeout under the fake LLM (target: instantaneous; real LLM latency is a future-spec problem).
- Re-run overwrites — the upsert path must be race-safe (`ON CONFLICT(extraction_id) DO UPDATE`), preserve `created_at`, and advance `updated_at`.
- Thesis-angle JSON envelope parsing is the use case's responsibility, not the LLM adapter's. The adapter returns raw text; structure validation lives in `application/analyzer`.
