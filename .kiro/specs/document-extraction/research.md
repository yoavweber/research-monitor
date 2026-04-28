# Research & Design Decisions

## Summary

- **Feature**: `document-extraction`
- **Discovery Scope**: Complex Integration — new aggregate, async job lifecycle, subprocess adapter to MinerU, restart recovery, must compose cleanly with existing `paper` / `source` aggregates without sharing schemas.
- **Key Findings**:
  - The existing hexagonal layout (`domain/<entity>` ports, `application/<entity>` use cases, `infrastructure/persistence/<entity>` adapters, `internal/http/route/<entity>_route.go`) already supplies a 1:1 template the extraction aggregate can mirror; no new architectural pattern is needed.
  - MinerU 3.x emits LaTeX math with `$...$` (inline) / `$$...$$` (display) by default and the delimiter set is configurable via `latex-delimiter-config`. The normalizer must therefore (a) treat `$/$$` as the canonical form, (b) defensively rewrite `\(...\)` and `\[...\]` if the host operator overrides the config, (c) be locked against a sample DeFi paper before code lands.
  - The async lifecycle has no in-repo precedent: every existing endpoint is synchronous. The simplest fit is a single in-process worker goroutine driven by a buffered signal channel (capacity-bounded; sends are non-blocking), with the `extractions` row as the durable source of truth so a dropped signal cannot lose work.
  - The storage-level uniqueness invariant on `(source_type, source_id)` reuses the same composite-`uniqueIndex` pattern already proven by `papers.Source/SourceID` (`gorm:"uniqueIndex:idx_papers_source_source_id"`); GORM's `TranslateError` flag (already enabled in bootstrap) surfaces the duplicate as `gorm.ErrDuplicatedKey` for race-safe overwrite-in-place.
  - Restart recovery has no incumbent design either; the chosen shape (synchronous `running → failed: process_restart` flip + per-pending self-signal before the worker goroutine launches) keeps the recovery contract verifiable without a polling ticker and without resurrecting in-flight extractor processes.

## Research Log

### MinerU CLI output and math delimiter format

- **Context**: The brief mandates math fidelity (`$...$` / `$$...$$`) and explicitly requires sample-verifying MinerU's delimiter behaviour during the design phase before the normalizer is locked.
- **Sources Consulted**:
  - MinerU output docs: <https://opendatalab.github.io/MinerU/reference/output_files/>
  - MinerU configuration reference: <https://deepwiki.com/opendatalab/mineru/9.3-configuration-file-reference>
  - MinerU project README: <https://github.com/opendatalab/MinerU>
- **Findings**:
  - Default markdown output wraps display formulas with `$$...$$` and inline formulas with `$...$`.
  - MinerU exposes `latex-delimiter-config` (in its config file) to override the delimiter set; an operator who sets this to `\(...\)` / `\[...\]` would produce a non-default but legal-LaTeX-Markdown variant.
  - Output is a **bundle directory** (one folder per input PDF) containing the markdown plus JSON sidecars and image dumps; only the `.md` is consumed by this spec.
- **Implications**:
  - Normalizer's contract: emit `$...$` / `$$...$$` regardless of input. v1 implementation: pass through default-formatted output untouched + defensive rewrite of `\(...\)` → `$...$` and `\[...\]` → `$$...$$` so a host with a customised MinerU config still satisfies the contract.
  - The bundle directory must be created under `os.TempDir()` per call and removed after the markdown has been read into memory; image and JSON sidecars are ignored.
  - **Follow-up locked into Task 1 of `tasks.md`**: run the `mineru`-tagged integration tests at `backend/tests/integration/extraction_mineru_adapter_test.go` and `backend/tests/integration/extraction_mineru_e2e_test.go` against the fixture PDF (`backend/tests/integration/testdata/amm_arbitrage_with_fees.pdf`). Both tests log the full markdown output regardless of pass / fail; the operator inspects the logged output, locks the normalizer rules and the adapter's CLI-error classification table against what MinerU actually emits, and only then ratchets the assertions from "expected fail" into the regression guard.

### Existing hexagonal patterns to mirror

- **Context**: The new aggregate must obey `structure.md`'s layering rules and reuse the conventions established by `paper` (composite-key persistence, dedupe via unique index) and `source` (CRUD use case, controller wiring).
- **Sources Consulted**:
  - `internal/domain/paper/{ports.go,model.go,errors.go}` — composite-key port shape, `*shared.HTTPError` sentinel pattern.
  - `internal/infrastructure/persistence/paper/{model.go,repo.go}` — `Paper` GORM model with `uniqueIndex:idx_papers_source_source_id`, `gorm.ErrDuplicatedKey` handling.
  - `internal/infrastructure/persistence/migrate.go` — central `AutoMigrate` registry.
  - `internal/http/route/{route.go,paper_route.go,source_route.go}` — route file shape, per-resource `XxxRouter(d Deps)` function, group mounted under `/api`.
  - `internal/bootstrap/app.go` — composition root, `route.Deps` shape, `shared.SystemClock{}` injection.
- **Findings**:
  - `paper.Repository` already takes `(source, sourceID)` as a composite key; it’s the same convention this spec consumes.
  - `paper.errors.go` declares `*shared.HTTPError` sentinels (`ErrNotFound`, `ErrCatalogueUnavailable`, etc.) and the `ErrorEnvelope` middleware translates them to wire status codes — extraction sentinels follow the same shape.
  - The `route.Deps` struct currently carries `Arxiv` and `Paper` config sub-bundles; extending it with an `Extraction` sub-bundle (worker handle, repo, use case, configurable thresholds) is the established extension point.
  - `tests/mocks/` already hosts hand-written fakes (`paper_fetcher.go`, `paper_repo.go`, `clock.go`, `logger.go`) so new doubles for `extraction.Extractor` etc. drop into the same directory.
- **Implications**:
  - No deviation from existing conventions is required.
  - Naming follows the same Go-idiomatic rule: callsite types are `extraction.UseCase`, `extraction.Repository`, `extraction.Extractor`; implementing structs are unexported (`extractionUseCase`, `repository`, `mineruExtractor`); constructors return the interface.

### Async job lifecycle in-process

- **Context**: MinerU latency is 30s–multi-minute; an HTTP request cannot block. No in-repo precedent for asynchronous work.
- **Sources Consulted**:
  - Existing in-process tickers: none. The arxiv use case is sync.
  - Steering `tech.md` (no message broker / Redis / queue in v1).
- **Findings**:
  - The simplest viable shape is one worker goroutine + a buffered `chan struct{}` wake signal. The DB row carries the full execution input (`request_payload` JSON), so the channel never carries data.
  - Non-blocking `select { case ch <- struct{}{}: default: }` send means a full buffer is benign — the worker re-checks the DB after every drained job and will discover unsent rows on the next signal.
  - Restart recovery decomposes into two synchronous bootstrap steps (run before worker goroutine launch): (1) `running → failed: process_restart`, (2) `SELECT id FROM extractions WHERE status='pending'` and self-signal once per row.
- **Implications**:
  - Worker lifecycle (`Start(ctx) → Stop(ctx)`) is wired in bootstrap with graceful-shutdown semantics; `ctx.Done()` interrupts the wake loop and any in-flight `Extractor.Extract` call.
  - No multi-worker concurrency is admitted in v1 — a future spec can lift the constraint by introducing an advisory-lock claim step before transitioning `pending → running`.

### Storage-level dedupe and overwrite-in-place

- **Context**: Re-extraction of an already-present `(source_type, source_id)` must overwrite the prior artifact while preserving the row id, and concurrent submissions must not leave two artifacts visible to readers.
- **Sources Consulted**:
  - `internal/infrastructure/persistence/paper/repo.go` (`Save` switch on `gorm.ErrDuplicatedKey`).
  - `internal/infrastructure/persistence/paper/model.go` (composite `uniqueIndex` tags).
  - `internal/bootstrap` — GORM is opened with `gorm.Config{TranslateError: true}` so unique-violations surface as `gorm.ErrDuplicatedKey` (paper repo comment confirms).
- **Findings**:
  - SQLite enforces composite `UNIQUE` indexes; GORM translates the constraint failure into `gorm.ErrDuplicatedKey`.
  - The repository can offer a single transactional `Upsert(ctx, payload)` method that (a) attempts `INSERT`, (b) on `ErrDuplicatedKey` performs `UPDATE … WHERE source_type = ? AND source_id = ?` resetting status, body, error fields, and `created_at` while preserving `id`. The operation is wrapped in a single transaction so concurrent calls serialise on the unique index and exactly one row is observable at any time.
- **Implications**:
  - Repository contract names this `Upsert` (or `EnqueueOrReextract`) so the use case does not need to perform read-then-write logic that races.
  - The operator log entry (`extraction.reextract`) is emitted by the use case after the repository confirms an overwrite (returning `(id, priorStatus, priorReason)` from the upsert path).

### Word-count gate as a post-extraction predicate

- **Context**: `Requirement 4.4` mandates a configurable `max_words` threshold that fails the extraction with reason `too_large`.
- **Findings**:
  - The threshold is checked **after** normalization but **before** persisting the artifact body, so an oversized extraction never costs a body write nor a partial artifact visible to a reader.
  - Word count is the same value reported in `metadata.word_count` on success — there is one canonical counter (whitespace-token split) shared between the gate and the artifact metadata.
- **Implications**:
  - Use-case order: dequeue → expiry check → invoke extractor → normalize → count words → if `count > max_words` mark `failed: too_large` with the count and threshold in the message; else persist body, mark `done`.

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| Hexagonal aggregate (chosen) | New `extraction` aggregate mirroring `paper` and `source`: domain ports + sentinels, application use case, GORM repo, MinerU adapter, in-process worker, HTTP controller. | Aligns with `structure.md` layering; reuses `*shared.HTTPError`, `Logger`, `Clock`, `APIToken` middleware; minimum new vocabulary. | Subprocess adapter is a new infrastructure shape (not yet present in repo); extra care needed around context cancellation and tempdir cleanup. | Selected. |
| Sync extraction in `arxiv` use case | Inline PDF download + extraction inside the existing arxiv ingest call. | Zero new wiring. | Violates 30s HTTP budget, blocks other ingest, no per-extraction lifecycle, couples arxiv-specific concerns to extraction contract. | Rejected: incompatible with MinerU latency. |
| External job queue (Redis / Asynq / SQS) | Run MinerU under a real distributed queue with retries, scheduling, dashboard. | Battle-tested retry/observability semantics; multi-worker for free. | Brings infrastructure (`tech.md` declares Postgres/Redis as deferred) and an operational footprint that exceeds the personal-monitor scope. | Rejected: scope explosion. |
| Long-lived `mineru-api` HTTP sidecar | Run MinerU's bundled HTTP server as a long-lived subprocess and call it via HTTP. | Eliminates per-call subprocess cold-start once the model is hot. | Adds a second binary / port to manage; MinerU 3.x already auto-spawns and reuses `mineru-api` under the CLI, so the practical gap is small; the `Extractor` port lets us migrate later without domain churn. | Deferred: keep v1 simple, migrate when latency profiling justifies it. |

## Design Decisions

### Decision: Async aggregate with a single in-process worker

- **Context**: PDF extraction takes 30s–several minutes; HTTP requests cannot block. v1 must not introduce external job-queue infrastructure.
- **Alternatives Considered**:
  1. Sync extraction inside the request — rejected (HTTP timeout).
  2. External queue (Redis/Asynq) — rejected (out-of-scope infra).
  3. In-process goroutine with buffered signal channel — chosen.
- **Selected Approach**: One worker goroutine started from bootstrap, woken by a buffered `chan struct{}` (capacity `extraction.signal_buffer`, default `10`); the DB row's `request_payload` JSON column is the durable source of truth for execution inputs.
- **Rationale**: Simplest shape that satisfies the latency profile, is observable through the existing structured logger, and survives restart via the `process_restart` flip + per-pending self-signal sequence. No new infra is introduced.
- **Trade-offs**: Single-worker throughput cap (one extraction at a time); restart loses the in-flight extractor process (`failed: process_restart` is the recovery path; caller re-POSTs).
- **Follow-up**: Multi-worker concurrency, automatic retry, and cancellation endpoints are explicit Out-of-Boundary items deferred to follow-up specs.

### Decision: `(source_type, source_id)` storage invariant via composite unique index

- **Context**: Requirement 6.3/6.4 demands at most one artifact per pair, race-safe under concurrent submissions.
- **Alternatives Considered**:
  1. Application-level locking (mutex / advisory lock) — rejected (extra moving part; doesn't survive crashes cleanly).
  2. Composite `UNIQUE` index + transactional upsert — chosen.
- **Selected Approach**: GORM `uniqueIndex:idx_extractions_source_source_id` on (`source_type`, `source_id`), repository `Upsert` runs `INSERT … ON CONFLICT` semantics inside a transaction, returning the prior status / reason for log emission.
- **Rationale**: Mirrors the existing `papers` invariant; SQLite enforces it at the storage layer; `gorm.Config{TranslateError: true}` already in bootstrap surfaces the conflict as `gorm.ErrDuplicatedKey`.
- **Trade-offs**: Re-extraction loses the prior body content (overwrite-in-place is the spec contract; no history table). Future "extraction history" features would need a separate audit table — out of scope.
- **Follow-up**: Document the operator-visible `extraction.reextract` log line shape so downstream observability tooling can assert it.

### Decision: Peek-then-claim worker flow (split `ClaimNextPending` into `PeekNextPending` + `ClaimPending`)

- **Context**: An earlier draft combined "transition to running" and "return next pending row" into a single `ClaimNextPending` call. The pickup-time expiry predicate was then evaluated *after* the row was already in `running`, forcing `MarkFailed(_, expired, …)` to accept either `pending` or `running` as a valid prior status — a precondition that became unreachable in practice and that masked an extra round-trip write on every expired row.
- **Alternatives Considered**:
  1. Combined `ClaimNextPending` + claim-then-evaluate-then-undo on expiry — rejected (extra UPDATE on every expired row, looser repository preconditions).
  2. Split into `PeekNextPending` (read-only) + `ClaimPending(id)` (transition `pending → running`) — chosen.
- **Selected Approach**: The worker peeks the next pending row, evaluates `clock.Now()` vs `peeked.CreatedAt + job_expiry`, and on overrun calls `MarkFailed(id, FailureReasonExpired, …)` directly from `pending`. On not-overrun, `ClaimPending(id)` transitions the row to `running` and `UseCase.Process` runs.
- **Rationale**: Tightens repository preconditions to one prior-status per method (uniform "predicate-in-`WHERE`" enforcement, zero-rows-affected → `ErrInvalidTransition`); never enters `running` for an expired row; keeps the failure taxonomy honest (an expired row was never "running and then failed", so its lifecycle log doesn't show a phantom `running` transition).
- **Trade-offs**: Two repository calls per non-expired pickup (peek + claim) vs one combined call. At single-worker scale and SQLite throughput this is negligible; the precondition simplification is worth more than the extra round-trip.
- **Follow-up**: When multi-worker concurrency is introduced in a future spec, `ClaimPending` is exactly the right seam to make atomic via `UPDATE … WHERE id=? AND status='pending' RETURNING …`. The peek-then-claim shape ports cleanly.

### Decision: Graceful shutdown leaves the row in `running`, recovered on next boot

- **Context**: When `ctx.Done()` fires while `UseCase.Process` is mid-extraction, `Extractor.Extract` returns a context error. The use case must decide what status the row ends up in. Misclassifying graceful shutdown as `extractor_failure` would pollute the failure taxonomy.
- **Alternatives Considered**:
  1. `MarkFailed(_, ExtractorFailure, "context cancelled")` — rejected (operator-initiated shutdown is not an extractor problem).
  2. Add a new `FailureReasonShutdown` — rejected (unnecessary taxonomy growth; the existing `process_restart` value already communicates "the process owning this run is gone").
  3. Leave the row in `running`; rely on `RecoverRunningOnStartup` at next boot to flip it to `failed: process_restart` — chosen.
- **Selected Approach**: When `Extract` returns `ctx.Err()`, the use case skips the `MarkFailed` write and exits. The row stays in `running` until the next process boot's startup recovery sequence transitions it to `failed: process_restart`.
- **Rationale**: Reuses the existing `process_restart` recovery path verbatim; preserves the single source of truth for "this row's worker is gone"; avoids a new `FailureReason` value; the bootstrap shutdown path already waits for the worker goroutine to finish before closing the DB, so the recovery flip is guaranteed to run before the next claim.
- **Trade-offs**: A row "stuck in running" is briefly visible to readers (`GET /api/extractions/:id`) during the shutdown window; the existing read contract surfaces `running` without artifact fields, which is correct for an in-flight job.
- **Follow-up**: Document this in the Worker Implementation Notes and in the state diagram caption so reviewers don't reintroduce a `MarkFailed` call here.

### Decision: AC 5.3 reconciliation favours AC 5.5 + the brief

- **Context**: AC 5.3 says was-running-at-restart rows produce the same `expired` outcome when the threshold is exceeded; AC 5.5 says was-running-at-restart rows always become `failed: process_restart`. A strict literal reading of both is contradictory.
- **Alternatives Considered**:
  1. Resurrect was-running rows back to `pending` at restart, then let the expiry predicate decide `expired` vs another claim — rejected (contradicts AC 5.5's literal text and the brief's explicit choice; brings phantom resurrection that the operator cannot distinguish from "still processing").
  2. AC 5.5 supersedes for was-running rows; AC 5.3's "expiry equivalence" applies only to rows currently in `pending` — chosen.
- **Selected Approach**: `RecoverRunningOnStartup` always flips was-running rows to `failed: process_restart` regardless of `created_at` age. The expiry predicate is the worker's pickup-time check on rows currently in `pending` (whether original `Submit` or post-`process_restart` re-`POST`). The reconciliation is documented in `design.md` under the lifecycle state diagram so it survives review.
- **Rationale**: Brief is explicit and unambiguous; calling a crashed-mid-run row `expired` would misattribute the cause; the operator's recovery action (re-`POST`) is identical for either outcome, so the distinction is purely about correct attribution in logs and reads.
- **Trade-offs**: A reviewer reading AC 5.3 in isolation may flag the deviation; the design's explicit reconciliation note is the mitigation.
- **Follow-up**: A future requirements pass may want to rewrite AC 5.3 to remove the ambiguity. Out of scope for this design.

### Decision: Pickup-time expiry only, no background sweeper

- **Context**: Requirement 5.4 forbids any scheduled sweep that transitions extractions to `expired` ahead of pickup.
- **Alternatives Considered**:
  1. Periodic ticker scanning the DB — rejected (forbidden).
  2. Pickup-time predicate inside the worker — chosen.
- **Selected Approach**: Immediately after the worker dequeues a row and before invoking the extractor, compare `clock.Now()` against `created_at + extraction.job_expiry`; on overrun, transition to `failed: expired` and skip the extractor.
- **Rationale**: Honours the requirement, removes a goroutine, keeps the worker as the only writer that demotes a `pending` row.
- **Trade-offs**: A `pending` row whose creation time is older than `job_expiry` keeps occupying disk until the worker eventually reaches it (dequeues are oldest-first). Acceptable for personal-monitor scale.

### Decision: Pure-domain normalizer (build, do not adopt)

- **Context**: The normalization contract (math delimiter unification, references stripping, table/image/figure-caption skipping, whitespace word-count, title heuristic) is project-specific.
- **Alternatives Considered**:
  1. Off-the-shelf markdown sanitizer — rejected (no library matches this exact contract; the rules are a domain-product decision, not a generic markdown concern).
  2. Hand-written rule set in `domain/extraction/normalize.go` — chosen.
- **Selected Approach**: Pure functions, no I/O, deterministic, fully unit-testable in isolation. Operates on a `string` markdown input and returns a normalized `string` plus the derived `Title` and `WordCount` values.
- **Rationale**: Math fidelity is the headline product invariant; the rules need to be auditable line-by-line and exercised against fixture papers in unit tests without subprocess setup.
- **Trade-offs**: Future rule changes require re-running the fixture suite; that's the intended workflow.

### Decision: MinerU 3.x with the `pipeline` backend (build-vs-adopt: adopt the binary, build the adapter)

- **Context**: v1 needs a math-faithful PDF→Markdown extractor without authoring a parser, on a developer laptop where the 2.2 GB VLM weights of MinerU's default `hybrid-auto-engine` backend are an unwelcome cost relative to the math-fidelity gain on standard LaTeX papers.
- **Alternatives Considered**:
  1. `github.com/ledongthuc/pdf` (the prior `tech.md` planned library) — rejected (no math fidelity, plain-text output only).
  2. Mathpix / Marker / Nougat — rejected (Mathpix paid; Marker/Nougat have heavier prerequisites and weaker math baseline as of MinerU 3.x).
  3. MinerU CLI with default `hybrid-auto-engine` backend — rejected for v1 (requires the `MinerU2.5-Pro-*` VLM weights, ~2.2 GB extra; slower per-document; the marginal math-fidelity gain is not material for the LaTeX-equation-heavy DeFi papers this spec targets).
  4. MinerU CLI with the `pipeline` backend (`mineru -b pipeline …`) — chosen.
- **Selected Approach**: Subprocess adapter under `infrastructure/extraction/mineru/` invoking `mineru -b pipeline -p <pdf> -o <tmpdir>` via `exec.CommandContext`, reading the produced bundle's `.md`, classifying CLI exit codes / stderr into typed domain errors, cleaning the temp dir. The `-b pipeline` flag is hard-coded into the adapter (not exposed in env config) so the markdown output shape is stable across hosts.
- **Rationale**: The pipeline backend uses PP-DocLayoutV2 for layout, unimernet for formula recognition (the same model MinerU 2.x shipped — well-tested LaTeX recognition), and paddleocr_torch for OCR. Total model footprint drops from ~4.5 GB (pipeline + VLM) to ~2.3 GB (pipeline only); pipx venv stays at 1.7 GB. The `Extractor` port still keeps a Mathpix, `hybrid-auto-engine`, or `mineru-api`-over-HTTP migration trivial — those would be sibling adapters under `infrastructure/extraction/`, swapped in via bootstrap wiring.
- **Trade-offs**: VLM-only edge cases (non-standard layouts, rotated equations, hand-drawn figures) may degrade with the pipeline backend. For the DeFi-paper corpus targeted by this spec the trade-off is acceptable; a future spec can reintroduce the VLM backend behind the same `Extractor` port if a target paper exposes a real fidelity gap.
- **Follow-up**: Sample-verify MinerU's actual stderr / exit-code format for `scanned_pdf` and `parse_failed` cases against fixture PDFs before locking the error-classification table. The `mineru`-tagged tests at `tests/integration/extraction_mineru_*_test.go` are the verification mechanism — they run against the pipeline backend specifically.

## Risks & Mitigations

- **Risk**: MinerU output drifts (delimiter format change, bundle layout change) on a future MinerU release. **Mitigation**: pin a known-good MinerU version range in operator docs; defensive normalizer (`\(...\)` / `\[...\]` rewrite) absorbs the most likely delimiter drift; integration test gated by build tag `mineru` exercises the real CLI in CI when explicitly opted in.
- **Risk**: A `running` extraction whose process is forcibly killed leaves a residual MinerU subprocess. **Mitigation**: `exec.CommandContext` propagates `ctx.Done()` to the subprocess; the bootstrap shutdown path cancels the worker context before exit. Rows still in `running` after a hard crash are flipped to `failed: process_restart` at next startup.
- **Risk**: A full signal-channel buffer drops a wake under bursty submissions. **Mitigation**: the worker re-queries the DB after every drained job and continues until no `pending` row remains, so a dropped signal cannot leave work permanently stranded.
- **Risk**: The chosen `request_payload` JSON encoding is opaque to ad-hoc SQL. **Mitigation**: payload fields (`source_type`, `source_id`) are also stored as first-class indexed columns; the JSON blob is only consulted by the worker for the full execution input. Mirrors the `Authors` / `Categories` precedent in `infrastructure/persistence/paper/model.go`.
- **Risk**: Re-extraction loses prior artifact content with no recovery. **Mitigation**: Documented as the spec's contract (Requirement 1.6); operator-visible log line preserves the prior status / reason for forensics; prior body is not retained — out of scope.

## References

- [MinerU output file reference](https://opendatalab.github.io/MinerU/reference/output_files/) — bundle layout and default markdown contents.
- [MinerU configuration reference (DeepWiki)](https://deepwiki.com/opendatalab/mineru/9.3-configuration-file-reference) — `latex-delimiter-config` semantics.
- [MinerU GitHub](https://github.com/opendatalab/MinerU) — license terms, CLI invocation surface.
- `backend/.kiro/steering/structure.md` — layering and naming rules followed verbatim.
- `backend/.kiro/steering/testing.md` — real-over-fake test policy applied to repo / use-case tests.
- `backend/internal/infrastructure/persistence/paper/{model.go,repo.go}` — composite-key + `gorm.ErrDuplicatedKey` precedent.
- `backend/internal/bootstrap/app.go` — composition root and `route.Deps` extension point.
