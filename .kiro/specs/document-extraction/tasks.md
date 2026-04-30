# Implementation Plan

- [x] 1. Foundation: extraction config, domain types and ports, persistence schema

- [x] 1.1 (P) Add the extraction configuration block to bootstrap env
  - Add `EXTRACTION_MAX_WORDS` (default 50000), `EXTRACTION_SIGNAL_BUFFER` (default 10), `EXTRACTION_JOB_EXPIRY` (default 1h, parsed as a duration), `MINERU_PATH` (default `mineru`), and `MINERU_TIMEOUT` (default 10m) to the viper-backed env loader, mirroring the existing arxiv config style
  - Reject startup when `EXTRACTION_JOB_EXPIRY` parses to zero or a negative duration, and when `EXTRACTION_MAX_WORDS` or `EXTRACTION_SIGNAL_BUFFER` are non-positive
  - Cover the new fields with env-loader unit tests asserting defaults apply when vars are absent and that malformed durations and non-positive integers fail-fast at boot
  - Observable: `task test ./internal/bootstrap/...` passes and the new env tests fail-fast on the rejection cases without producing a partially-built `Env`
  - _Requirements: 4.4, 5.2, 6.6_
  - _Boundary: bootstrap env_

- [x] 1.2 (P) Define the extraction domain value types
  - Declare `Artifact`, `Metadata`, `RequestPayload`, `Failure`, `Extraction` aggregate, `JobStatus` constants (`pending`, `running`, `done`, `failed`), `FailureReason` constants (`scanned_pdf`, `parse_failed`, `extractor_failure`, `too_large`, `expired`, `process_restart`), `NormalizedArtifact`, `ExtractInput`, `ExtractOutput`, and `PriorState`
  - Add a package-level doc comment that names the aggregate's purpose and its `(source_type, source_id)` keying convention
  - Observable: the package compiles standalone and a value-types unit test enumerates exactly the four declared `JobStatus` constants and the six declared `FailureReason` constants, failing if either set drifts
  - _Requirements: 4.5, 5.1_
  - _Boundary: domain extraction model_

- [x] 1.3 (P) Declare the extraction error sentinels
  - Define `ErrInvalidRequest` (400), `ErrUnsupportedSourceType` (400), `ErrNotFound` (404), `ErrCatalogueUnavailable` (500), `ErrInvalidTransition` (500), `ErrScannedPDF`, `ErrParseFailed`, `ErrExtractorFailure` as `*shared.HTTPError` values, mirroring the `paper.errors.go` shape
  - Observable: a unit test wraps each sentinel via `fmt.Errorf("%w: ...")` and confirms `errors.As` recovers it and `shared.AsHTTPError` returns the expected status code
  - _Requirements: 1.3, 2.5, 4.1, 4.2, 4.3, 6.6_
  - _Boundary: domain extraction errors_

- [x] 1.4 Declare the extraction ports and submission request DTO
  - Author the `Repository`, `UseCase`, and `Extractor` interfaces matching the design's service-interface code blocks verbatim, with `context.Context` as the first parameter on every method
  - Author `SubmitRequest` with a `Validate() error` method that rejects empty `source_type` / `source_id` / `pdf_path` and any `source_type` other than `paper`
  - Observable: `Validate()` is exercised by a table-driven unit test covering each rejection case; the table asserts `errors.Is(err, ErrInvalidRequest)` or `errors.Is(err, ErrUnsupportedSourceType)` per case
  - _Requirements: 1.3, 6.1_

- [x] 1.5 (P) Implement the extractions persistence model and register the migration
  - Define the GORM `Extraction` row matching the design's physical schema: composite `uniqueIndex:idx_extractions_source_source_id` on `(source_type, source_id)`, composite `index:idx_extractions_status_created_at` on `(status, created_at)`, JSON-encoded `request_payload`, default-empty body / metadata / failure columns, and `TableName() = "extractions"`
  - Implement `FromDomain` (UUID assigned here, `Authors`/`Categories`-style JSON marshal of `request_payload`) and `ToDomain` round-tripping every column
  - Append `&extraction.Extraction{}` to `persistence.AutoMigrate`
  - Observable: a migration test runs `AutoMigrate` against an in-memory SQLite, queries `sqlite_master`, and confirms both indexes exist with the expected column ordering
  - _Requirements: 6.3, 6.5, 6.6_
  - _Boundary: infrastructure persistence extraction_
  - _Depends: 1.2_

- [x] 2. MinerU verification gate: subprocess adapter + sample-locked normalizer contract

- [x] 2.1 Implement the MinerU subprocess adapter
  - Construct the adapter via `NewMineruExtractor(path, timeout)` returning `extraction.Extractor`
  - Per-call: create a fresh temp directory under `os.TempDir()`, derive a child `context.WithTimeout` from the call-site `ctx` using `mineruTimeout`, invoke `exec.CommandContext(ctx, mineruPath, "-b", "pipeline", "-p", pdfPath, "-o", tmpDir)` (the `-b pipeline` flag is mandatory; pins the adapter to the pipeline backend so no VLM weights are required), read the produced bundle's single `.md` file into memory, and clean up the temp directory in a deferred call that runs on every return path including panics
  - Implement the design's CLI-error classification table mapping to `ErrScannedPDF`, `ErrParseFailed`, `ErrExtractorFailure`, with `ctx.Err()` returned as-is for cancellation
  - Observable: the package compiles, the constructor returns a value assignable to `extraction.Extractor`, and an isolated smoke run (without real MinerU) confirms that an `exec.CommandContext` against a non-existent binary surfaces as `ErrExtractorFailure` (no raw `os/exec` error leakage)
  - _Requirements: 4.1, 4.2, 4.3, 6.1, 6.2_
  - _Boundary: infrastructure extraction mineru_
  - _Depends: 1.1, 1.3, 1.4_

- [x] 2.2 Author the mineru-tagged adapter integration test
  - Place the test under `tests/integration/extraction_mineru_adapter_test.go` with a `//go:build mineru` directive
  - Resolve the fixture at runtime via `wd, _ := os.Getwd(); pdfPath := filepath.Join(wd, "testdata", "amm_arbitrage_with_fees.pdf")` (no hardcoded absolute path)
  - Construct the MinerU adapter via its public constructor, assign to a variable typed as `extraction.Extractor`, and call `Extract(ctx, ExtractInput{PDFPath: pdfPath})` directly
  - Use `t.Logf("%s", output.Markdown)` to dump the full markdown body to test output unconditionally so the operator can inspect what MinerU actually produces; commit the test with the design's expected normalizer assertions even though they are expected to fail until Task 2.3 ratchets them
  - Observable: `go test -tags mineru ./tests/integration/ -run TestMineruAdapter -v` against a host with `mineru` installed runs the adapter, prints the markdown body, and the test exits (passing or failing) inside the per-call MinerU timeout
  - _Requirements: 6.2_
  - _Depends: 2.1_

- [x] 2.3 Sample-verify MinerU output and lock the normalizer contract
  - Run the test from 2.2 against the fixture on a host with MinerU installed; capture the logged markdown body
  - Compare the observed output to the design's normalizer rules: math delimiter format (`$/$$` vs `\(...\)` / `\[...\]`), heading levels and exact text used for references / bibliography / works-cited sections, table syntax (GFM pipes vs HTML), image lines, figure-caption prefix patterns
  - If observed output matches the design contract: ratchet the assertions in `extraction_mineru_adapter_test.go` from "expected fail" placeholders into the regression guard so the test now passes under `-tags mineru`
  - If observed output deviates from the design contract: STOP, update `design.md`'s normalizer Implementation Notes (and `research.md`'s sample-verification follow-up) to record the actual MinerU output shape, then re-run this task before proceeding
  - Observable: `go test -tags mineru ./tests/integration/ -run TestMineruAdapter` passes on the verifying host; design.md's normalizer rules and the test assertions agree on which delimiter format, heading text, and skip-block patterns are authoritative
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7, 3.8, 6.2_
  - _Depends: 2.2_

- [x] 3. Core implementation: normalizer, repository, use case, worker, controller, route

- [x] 3.1 (P) Implement the pure-domain normalizer
  - Author `Normalize(markdown, fallbackTitle string) NormalizedArtifact` as a pure function (no I/O, no time, no random)
  - Implement: math delimiter rewrite (`\(...\) → $...$`, `\[...\] → $$...$$`, `$/$$` pass-through); references / bibliography / works-cited tail truncation (case-insensitive, exact heading text match across `#` through `######`); GFM-table / image-line / figure-caption skipping; whitespace collapse after removals; first `#` heading title selection with `fallbackTitle` fallback; `len(strings.Fields(body))` word count
  - Cover every rule in a table-driven unit test, plus the no-references-heading case (no truncation), the no-`#`-heading case (fallback used), and a body with mixed-whitespace runs to exercise the word-count contract
  - Observable: `task test ./internal/domain/extraction/...` passes the normalizer suite; the suite includes an assertion that re-running `Normalize` on its own output is a fixed point for a representative fixture
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_
  - _Boundary: domain extraction normalize_
  - _Depends: 2.3_

- [x] 3.2 (P) Implement the GORM-backed extraction repository
  - Implement `Upsert` (insert-then-on-conflict-overwrite using `gorm.ErrDuplicatedKey`, refreshing `created_at` and clearing body / failure columns, capturing prior `(status, failure_reason)` as `*PriorState` inside the same transaction); `FindByID`; `PeekNextPending` (pure SELECT, no UPDATE); `ClaimPending` (UPDATE with `WHERE id=? AND status='pending'`, zero-rows-affected → `ErrInvalidTransition`); `MarkDone` (UPDATE with `WHERE id=? AND status='running'`); `MarkFailed` (UPDATE with reason-conditional WHERE: `status='pending'` for `FailureReasonExpired`, `status='running'` otherwise); `RecoverRunningOnStartup` (idempotent flip of every `running` row to `failed: process_restart`); `ListPendingIDs`
  - Map `gorm.ErrRecordNotFound` to `ErrNotFound`, all other DB errors to wrappings of `ErrCatalogueUnavailable`
  - Real-DB tests over `:memory:` SQLite per `testing.md`: insert success, conflict overwrite returning the captured `PriorState`, peek-without-transition (asserted by re-reading the row's status after peek), claim-from-pending success, claim-from-running `ErrInvalidTransition`, `MarkFailed(_, FailureReasonExpired, _)` accepts pending and rejects running, `MarkFailed` for any other reason rejects pending, `RecoverRunningOnStartup` is idempotent across two consecutive calls, `request_payload` JSON round-trips losslessly, `FindByID` on a missing id returns `ErrNotFound`
  - Observable: `task test ./internal/infrastructure/persistence/extraction/...` passes the entire suite
  - _Requirements: 1.5, 1.6, 2.1, 2.7, 5.1, 5.5, 5.7, 6.3, 6.4, 6.6_
  - _Boundary: infrastructure persistence extraction_
  - _Depends: 1.5_

- [x] 3.3 Implement the extraction use case
  - Author `extractionUseCase` taking `Repository`, `Extractor`, `shared.Logger`, `shared.Clock`, and a `chan<- struct{}` wake channel via the constructor
  - `Submit`: re-validate `source_type == "paper"` (mirrors the controller's `Validate` for non-HTTP entrypoints), call `Repository.Upsert`, on `*PriorState` non-nil emit a structured `extraction.reextract` log line carrying id / source key / prior status / prior failure reason, then a non-blocking `select` send on the wake channel
  - `Process`: invoke `Extractor.Extract`, on `ctx.Err()` return without writing (row stays in `running` for next-boot recovery), otherwise call `Normalize` with the source filename basename as `fallbackTitle`, apply the `max_words` gate (`failed: too_large` carrying the actual count and configured threshold in the message), populate `Artifact.Metadata.ContentType` from the request `source_type`, and call `MarkDone` or `MarkFailed` with the centralised error-to-`FailureReason` mapping (`ErrScannedPDF → scanned_pdf`, `ErrParseFailed → parse_failed`, `ErrExtractorFailure → extractor_failure`)
  - `Get`: read via `Repository.FindByID`
  - Add the `extraction.Extractor` fake (`tests/mocks/extraction_extractor.go`) following the canonical `paper_fetcher.go` shape (zero-value usable, exported fields, `sync.Mutex`, recorded calls)
  - Use case unit tests with real GORM-over-`:memory:` repo + the fake Extractor + a frozen `shared.Clock`: Submit creates a new row; Submit overwrites a prior row, refreshes `created_at`, and emits exactly one `extraction.reextract` log line carrying the prior status; Submit rejection on `source_type=html` returns `ErrUnsupportedSourceType`; Process happy path → done with `metadata.content_type=="paper"` and the title pulled from the markdown's first `#`; each `Extractor` error maps to its corresponding `FailureReason`; `word_count > max_words` → `failed: too_large` carrying both numbers in the message; `ctx` cancellation mid-`Process` leaves the row in `running` and emits no `MarkFailed` write
  - Observable: `task test ./internal/application/extraction/...` passes the use case suite
  - _Requirements: 1.1, 1.2, 1.3, 1.5, 1.6, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 3.6, 3.8, 4.1, 4.2, 4.3, 4.4, 4.5, 5.7_
  - _Depends: 3.1, 3.2_

- [x] 3.4 Implement the worker and its critical-path tests
  - Author `Worker` owning the wake channel handle (constructor takes `extraction.UseCase`, `shared.Logger`, `shared.Clock`, `<-chan struct{}` receive end, plus the configured `job_expiry` duration); expose `Start(ctx)` (launches a single goroutine non-blocking) and `Stop()` (blocks until the goroutine exits)
  - Goroutine loop: receive a wake → enter drain loop calling `PeekNextPending`; for each peeked row, evaluate `clock.Now() >= peeked.CreatedAt + job_expiry` and on overrun call `Repository.MarkFailed(id, FailureReasonExpired, msg)` from `pending`; otherwise call `Repository.ClaimPending(id)` then `UseCase.Process(ctx, row)`; continue draining until peek returns empty; on `ctx.Done()` return without writing any expiry / failure for the in-flight row
  - Extend the `extraction.Extractor` fake (or add a sibling test helper) so it can block on a per-test channel until cancellation is observed — required by the worker shutdown test
  - Worker unit tests with real GORM-over-`:memory:` repo + the fake use case and / or fake Extractor:
    - **peek-then-claim sequence**: not-expired branch calls `ClaimPending` then `Process` exactly once per pending row; drain loop terminates when peek returns empty
    - **expiry peek (Critical Issue 2 resolution)**: insert a row with `CreatedAt = clock.Now() - 2 * job_expiry`, fire one wake; assert the row transitions directly from `pending` to `failed: expired` with zero calls on the fake Extractor and no intermediate `running` snapshot
    - **non-blocking send under bursty submit**: fill the wake channel and call the use case's `Submit` repeatedly; assert `Submit` never blocks and the worker still drains every committed row
    - **worker shutdown (Critical Issue 1 resolution)**: arrange the fake Extractor to block until the test signals; trigger one wake so the worker enters `Process`; cancel the worker `ctx`; assert the row is still in `running` immediately after `Stop()` returns; then call `Repository.RecoverRunningOnStartup` and assert the row is now in `failed: process_restart`
  - Observable: `task test ./internal/application/extraction/...` passes every worker test including the two critical-issue-resolution cases
  - _Requirements: 1.2, 5.1, 5.2, 5.3, 5.4, 5.5, 5.6_
  - _Depends: 3.2, 3.3_

- [x] 3.5 (P) Implement the HTTP controller, wire DTOs, and OpenAPI annotations
  - Author `SubmitExtractionRequest` (json tags + binding rules for `source_type`, `source_id`, `pdf_path`) with `Validate() error`; author `ExtractionStatusResponse` plus envelope wrappers (`ExtractionStatusEnvelope`) using `omitempty` so artifact fields render only when `status == "done"` and failure fields render only when `status == "failed"`
  - Author `ExtractionController.Submit` (`POST /api/extractions` → `202` with `{id, status: "pending"}`) and `ExtractionController.Get` (`GET /api/extractions/:id` → `200` with the conditional response body); surface domain sentinels via `c.Error` for the existing `ErrorEnvelope` middleware
  - Add swag annotations covering `@Summary`, `@Tags Extractions`, `@Accept json`, `@Produce json`, `@Param`, `@Success 202`, `@Success 200`, `@Failure 400`, `@Failure 401`, `@Failure 404`, `@Failure 500` (each `{object} common.ErrorEnvelope`), `@Security APIToken`, `@Router /extractions ...` per the existing paper / arxiv controllers; run `task swag` and confirm `docs/` regenerates with both endpoints
  - Add the `extraction.UseCase` fake (`tests/mocks/extraction_usecase.go`) following the canonical fake shape (zero-value usable, `sync.Mutex`, recorded calls, exported queued return values)
  - Controller unit tests against the fake UseCase: `202` on Submit with a valid body; `400` on missing `pdf_path`; `400` on `source_type=html` via `ErrUnsupportedSourceType`; `200` on `done` includes `title`, `body_markdown`, `metadata`; `200` on `failed` includes `failure_reason`, `failure_message` and omits artifact fields; `200` on `pending` / `running` omits both artifact and failure fields; `404` on `ErrNotFound`
  - Observable: `task test ./internal/http/controller/extraction/...` passes; `task swag` regenerates `docs/` and the diff shows the new endpoints with the documented `@Failure` rows
  - _Requirements: 1.1, 1.3, 2.1, 2.2, 2.3, 2.4, 2.5_
  - _Boundary: interface http extraction controller_
  - _Depends: 1.4_

- [x] 3.6 Wire ExtractionRouter and extend route.Deps
  - Add `ExtractionConfig` (carrying the persisted `extraction.Repository`, the `extraction.UseCase`, and a handle for the worker so route-level smoke tests can inspect it) to `route.Deps`
  - Author `ExtractionRouter(d Deps)` that constructs `ExtractionController` from `d.Extraction.UseCase` and registers `POST /api/extractions` and `GET /api/extractions/:id` under the `/api` group (the `APIToken` middleware is already mounted there)
  - Call `ExtractionRouter(d)` from `route.Setup`
  - Add a route-level smoke test asserting both endpoints are registered (returning a non-404 path-match status) and that each rejects requests missing `X-API-Token` with `401`
  - Observable: `task test ./internal/http/route/...` passes the new smoke test; the controller is reachable through the registered routes
  - _Requirements: 1.4, 2.6_
  - _Depends: 3.5_

- [x] 4. Compose extraction in bootstrap and validate startup recovery
  - Wire `bootstrap/app.go`: build `mineruExtractor` from env (`MINERU_PATH`, `MINERU_TIMEOUT`), construct the `extraction.Repository`, allocate the buffered wake channel sized by `EXTRACTION_SIGNAL_BUFFER`, build `extractionUseCase` (passing the send-side of the channel), build `Worker` (passing the receive-side, `EXTRACTION_JOB_EXPIRY`, the logger and `shared.SystemClock{}`), populate `route.Deps.Extraction`
  - Run `Repository.RecoverRunningOnStartup(appCtx)` BEFORE registering routes; abort `NewApp` with a wrapped error if it fails (Requirement 6.6)
  - Enumerate `Repository.ListPendingIDs(appCtx)` and perform one non-blocking `select` send per id on the wake channel; only then call `Worker.Start(appCtx)` so the goroutine drains pre-existing pending rows without operator action
  - Add a `Stop()` hook to the app's shutdown sequence that cancels `appCtx`, calls `Worker.Stop()`, and only then closes the DB so an in-flight `Process` returns before the connection goes away
  - Bootstrap unit test seeds the DB with one `running` row and one `pending` row before calling `NewApp`; asserts post-construction (a) the `running` row is now `failed: process_restart`, (b) the `pending` row is still `pending`, (c) the wake channel has buffered exactly one signal corresponding to the seeded pending row, and (d) `Worker.Stop()` returns within the test deadline
  - Observable: `task test ./internal/bootstrap/...` passes the new test; `task run` boots cleanly with the extraction routes mounted under `/api`
  - _Requirements: 1.4, 2.6, 5.5, 5.6, 6.5, 6.6_
  - _Depends: 2.1, 3.2, 3.4, 3.6_

- [x] 5. Validation: hermetic integration and MinerU end-to-end

- [x] 5.1 (P) Author the hermetic extraction integration suite
  - Extend `tests/integration/setup.SetupTestEnv` so the test harness can inject a hand-written fake `extraction.Extractor` (mirrors the existing fake-fetcher injection pattern); the fake returns scripted `ExtractOutput` / typed-error sequences per test
  - Place the suite at `tests/integration/extraction_test.go` with `//go:build integration`
  - Cases: `401` without `X-API-Token`; `400` on missing `pdf_path`; `400` on `source_type=html`; `404` on `GET` with an unknown id; happy path POST → poll until `done` with the fake returning a markdown body that exercises math delimiters and a references heading (verifying normalization end-to-end); re-extraction overwrites in place and the second `GET` returns the new artifact while the row id is unchanged; oversized body with `EXTRACTION_MAX_WORDS=1` → `failed: too_large` with the actual count and threshold in the message; scanned-PDF fake returning `ErrScannedPDF` → `failed: scanned_pdf`
  - Observable: `go test -tags integration ./tests/integration/ -run TestExtraction -v` passes every case under 30 seconds with no MinerU dependency
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 3.3, 3.6, 3.8, 4.1, 4.4, 4.5, 5.7_
  - _Boundary: tests integration hermetic_
  - _Depends: 4_

- [x] 5.2 (P) Author the mineru-tagged end-to-end test and ratchet final assertions
  - Place the test at `tests/integration/extraction_mineru_e2e_test.go` with `//go:build mineru`
  - Resolve the fixture at runtime via `wd, _ := os.Getwd(); pdfPath := filepath.Join(wd, "testdata", "amm_arbitrage_with_fees.pdf")` — never a hardcoded absolute path
  - Boot `SetupTestEnv` configured to wire the **real** MinerU adapter (env-driven), POST `/api/extractions` with `{ "source_type": "paper", "source_id": "amm-arbitrage-fees", "pdf_path": pdfPath }`, poll `GET /api/extractions/:id` every 2 seconds until `status == "done"` or the 5-minute deadline expires; on timeout fail with the last observed row state
  - Use `t.Logf("%s", body_markdown)` to print the final markdown body to test output regardless of pass / fail so the operator can audit what the full pipeline produced end-to-end
  - Initial assertions, expected to fail until ratcheted: `title` non-empty; `body_markdown` contains at least one match for `\$[^\$]+\$` OR `\$\$[^\$]+\$\$`; `body_markdown` contains no heading whose trimmed text matches `references` / `bibliography` / `works cited` (case-insensitive across `#` through `######`); `metadata.word_count > 0` AND `metadata.word_count <= EXTRACTION_MAX_WORDS`; `metadata.content_type == "paper"`
  - Run the test against a host with MinerU installed; ratchet assertions to match observed reality (or update normalizer / design and re-run) until the test passes — both outcomes lock the end-to-end contract
  - Observable: `go test -tags mineru ./tests/integration/ -run TestMineruE2E -v` passes within 5 minutes against the fixture; the logged `body_markdown` satisfies all five asserted invariants
  - _Requirements: 1.1, 1.2, 2.1, 2.2, 3.1, 3.3, 3.5, 3.6, 3.7, 3.8, 6.1, 6.2_
  - _Boundary: tests integration mineru e2e_
  - _Depends: 4_

## Implementation Notes

- **Task 1.5 follow-up**: the `ToDomain` malformed-`request_payload` JSON path wraps as `extraction.ErrCatalogueUnavailable` in code but is not yet exercised by a test. Add a malformed-JSON test alongside the repository tests in Task 3.2, where the read-side error path is naturally exercised end-to-end.
- **Worktree**: foundation phase committed on branch `worktree-document-extraction` at `backend/.claude/worktrees/document-extraction`. Spec files were committed on `main` as `a4a27d0` and fast-forwarded into the worktree branch; pre-existing uncommitted changes on `main` are unrelated to this spec and were not touched.
- **MinerU adapter backend choice**: the design and tasks pin `mineru -b pipeline -p ... -o ...` (not the default `hybrid-auto-engine`). Rationale captured in `research.md` "Decision: MinerU 3.x with the pipeline backend". The VLM model weights (~2.2 GB) are intentionally NOT downloaded; only the pipeline weights at `~/.cache/huggingface/hub/models--opendatalab--PDF-Extract-Kit-1.0` are required.
