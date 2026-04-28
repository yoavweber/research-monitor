# Requirements Document

## Project Description (Input)

**Who has the problem.** The researcher running the personal DeFi research monitor — sole user of the backend. Papers arriving through the existing `arxiv-fetcher` + `paper-persistence` pipeline are useful as catalogue entries but not as research material; their PDF bodies, where the math, methods, and arguments live, are unreachable for downstream LLM summarization and thesis-angle analysis.

**Current situation.** `paper.Entry` carries `PDFURL` but the PDF body is never fetched, stored, or normalized. `product.md` lists "extract body (HTML / PDF)" as a pipeline stage with no implementation. There is no extraction port, adapter, or persistence layer. The system has no concept of an asynchronous job: every existing HTTP endpoint completes synchronously, which is incompatible with the 30-second-to-multi-minute latency profile of a math-faithful PDF extractor.

**What should change.** Introduce a new `extraction` aggregate that converts a local PDF into a normalized markdown artifact (math preserved as `$...$` / `$$...$$`, references stripped, tables / images / figure captions skipped, whitespace word count, title from first `#` heading), persisted in storage separate from `papers`. The aggregate is content-agnostic and decoupled from `paper`: it consumes a PDF path plus a `(source_type, source_id)` key supplied by the caller, and emits an artifact whose metadata block carries only what the extraction itself produced (`content_type`, `word_count`). Anything that lives on the source paper (URLs, fetch timestamps) is the catalogue's concern, not this aggregate's. v1 only accepts `source_type = "paper"`.

The aggregate ships:

- A pluggable v1 `Extractor` impl backed by **MinerU** (subprocess CLI), swappable behind the port.
- A pure-domain normalizer applying the rules above deterministically.
- Persistence with one artifact per `(source_type, source_id)` keyed identity, the storage invariant enforced at the storage layer so concurrent submissions cannot produce duplicates.
- An asynchronous job lifecycle (`pending → running → done | failed`) driven by an in-process worker, with a configurable per-job expiry checked at pickup time (`extraction.job_expiry`, default `1h`) and a configurable post-extraction word-count gate (`extraction.max_words`, default `50000`). No background sweeper; expiry is a pickup-time predicate only.
- Restart recovery: on startup, any extraction whose status was `running` is transitioned to `failed: process_restart` before new work is admitted, and `pending` rows that survived the restart are picked up without operator intervention.
- HTTP endpoints under the authenticated `/api` group: `POST /api/extractions` (enqueue / re-extract; returns `202 Accepted` with `{ id, status }`) and `GET /api/extractions/:id` (status + artifact when `done`).
- Re-extraction overwrites the prior artifact in place, refreshes timing so a new expiry window applies, retains the row identifier, and emits an operator-visible log entry naming the prior status.

**Boundaries.** This spec stops at "normalized markdown is queryable". PDF acquisition / download, HTML extraction, OCR, table / image preservation, batch fan-out, LLM summarization, webhooks, automatic retry, multi-worker concurrency, cancellation endpoints, background expiry sweepers, and housekeeping are explicitly Out and reserved for follow-up specs. The link between an `extraction` row and a `papers` row is the `(source_type, source_id)` convention only; no foreign key, no schema sharing, no modification to `paper-persistence` or `arxiv-fetcher`.

**Constraints.** Go 1.25 + GORM v2 + SQLite (existing stack). Hexagonal layering per `structure.md`: `domain/extraction/` may not import `infrastructure/`, conversion via `ToDomain` / `FromDomain`. Naming follows the `source` / `paper` precedent (port `Repository`, unexported impl, `NewRepository(db)` returns the interface). `context.Context` is the first parameter of every port method, including `Extractor.Extract`. `log/slog` only via `shared.Logger`. Hand-written fakes under `tests/mocks/`. The `mineru` CLI math-delimiter format must be sample-verified during the design phase before the normalizer is locked. Operator prerequisite: `mineru` installed and reachable on the host.

Full discovery output, including approach rationale (Approach A + async), Boundary Candidates, Upstream / Downstream, and Existing Spec Touchpoints, is in `brief.md` adjacent to this file.

## Introduction

The document-extraction feature gives the research monitor an asynchronous, math-faithful path from a local PDF to a normalized markdown artifact that downstream LLM summarization can consume. It owns: the API contract for submitting extractions and polling their status, the normalization rules that make every artifact structurally predictable regardless of the underlying tool, the failure taxonomy that distinguishes content problems (scanned, too large) from infrastructure problems (extractor crash, expiry, restart), the job lifecycle and the operator-visible guarantees that go with it (restart recovery, pickup-time expiry, idempotent re-extraction), and a pluggable extractor port whose v1 implementation is MinerU. It does not own PDF acquisition, the link to `paper.Entry`, or anything past the markdown artifact in the pipeline.

## Boundary Context

- **In scope**:
  - An asynchronous request / poll API for converting a PDF on disk into a normalized markdown artifact, keyed by `(source_type, source_id)` supplied by the caller.
  - Markdown normalization: math delimiters (`$...$` inline, `$$...$$` display), references / bibliography / works-cited stripping, table / image / figure-caption skipping, whitespace word count, title selection (first `#` heading, fallback to source filename).
  - A typed failure taxonomy: `scanned_pdf`, `parse_failed`, `extractor_failure`, `too_large`, `expired`, `process_restart`, surfaced through the GET response.
  - A configurable post-extraction word-count gate (`extraction.max_words`, default `50000`).
  - A configurable per-job expiry (`extraction.job_expiry`, default `1h`), evaluated at the moment processing begins.
  - Restart recovery: in-flight extractions transition to `failed: process_restart` before new work is admitted; pending extractions resume without operator intervention.
  - One artifact per `(source_type, source_id)`, with the storage layer enforcing uniqueness even under concurrent submissions; re-submission overwrites the prior artifact in place and emits an operator-visible log entry.
  - A pluggable extractor contract; v1 ships MinerU as the default implementation.
  - Two authenticated HTTP endpoints under `/api`: `POST /api/extractions` and `GET /api/extractions/:id`.
  - A fail-fast startup contract: the service refuses to accept requests if the extraction catalogue's storage surface is not ready.
- **Out of scope**:
  - PDF acquisition or download. Callers supply a local `pdf_path`; the service does not fetch, copy, or stage PDFs.
  - HTML, OCR / scanned-PDF, or any non-PDF source format.
  - Preservation of tables, images, figure captions, equation rendering, or multi-column layout fixes beyond what the underlying extractor emits natively.
  - Batch / fan-out submission ("extract every paper from yesterday's fetch"); LLM summarization, thesis-angle flagging, or triage; webhooks and push notifications.
  - Automatic retry of failed extractions, multi-worker concurrency, cancellation endpoints, background expiry sweepers, auto-purge of completed rows.
  - Any modification to `paper-persistence`, `arxiv-fetcher`, or the `paper.Entry` shape.
  - A `source_url` or `fetch_date` field in the artifact metadata: those are properties of the source paper and live in `paper-persistence`; the extraction artifact intentionally does not duplicate them.
- **Adjacent expectations**:
  - The authenticated `/api` group mounts the existing static-token middleware (`X-API-Token`); this spec inherits that protection rather than reauthoring it.
  - The `(source_type, source_id)` pair on every extraction is the same identifier used by `paper-persistence`. The relationship between an extraction and a `papers` row is by convention only — there is no foreign key, no schema sharing, and no requirement that a corresponding `papers` row exist. v1 only accepts `source_type = "paper"`.
  - The caller is responsible for ensuring `pdf_path` refers to a file readable by the running service at the moment processing begins. The service does not validate PDF readability at submission time; failures to read or parse the file at extraction time surface as one of the typed failure reasons defined in Requirement 4.
  - The host operator is responsible for installing the underlying extractor tool (v1: `mineru`) and ensuring it is reachable on the host. The service does not bundle or install extractor binaries.
  - The `arxiv-fetcher` and `paper-persistence` boundaries are unchanged by this spec.

## Requirements

### Requirement 1: Submit an asynchronous extraction request

**Objective:** As the researcher, I want to submit a PDF for extraction and get back an identifier immediately while the actual work happens in the background, so that I can keep working without holding an HTTP connection open for a multi-minute extractor and can poll for the result later.

#### Acceptance Criteria

1. When an authenticated client issues `POST /api/extractions` with a body containing `source_type`, `source_id`, and `pdf_path`, the document-extraction service shall return an HTTP 202 response carrying a stable extraction identifier and an initial status of `pending`.
2. When the document-extraction service accepts a `POST /api/extractions` request, it shall return its response without waiting for the underlying extractor to run and without holding the connection open for the duration of extraction work.
3. If the request body omits `source_type`, `source_id`, or `pdf_path`, or if `source_type` is any value other than `paper` in v1, the document-extraction service shall return an HTTP 400 response and shall not enqueue an extraction.
4. If the request omits or presents an invalid `X-API-Token`, the document-extraction service shall return an HTTP 401 response and shall not enqueue an extraction.
5. When `POST /api/extractions` is accepted for a `(source_type, source_id)` pair that is not yet present in the catalogue, the document-extraction service shall create a new extraction with a stable identifier and an initial status of `pending`.
6. When `POST /api/extractions` is accepted for a `(source_type, source_id)` pair that is already present in the catalogue, the document-extraction service shall replace the prior artifact's content in place, reset the extraction's status to `pending`, refresh its creation timestamp so the new request gets a full expiry window, retain its identifier, and record an operator-visible event naming the prior status and reason.

### Requirement 2: Retrieve extraction status and artifact

**Objective:** As the researcher, I want a single read endpoint that returns the current state of an extraction and the extracted markdown when ready, so that I can poll the same URL until completion and consume the artifact in one place.

#### Acceptance Criteria

1. When an authenticated client issues `GET /api/extractions/:id` for an identifier that exists in the catalogue, the document-extraction service shall return an HTTP 200 response carrying the extraction's current status and the originating `(source_type, source_id)` pair.
2. When `GET /api/extractions/:id` is called and the extraction's status is `done`, the response shall additionally include `title`, `body_markdown`, and a `metadata` object containing `content_type` and `word_count`.
3. When `GET /api/extractions/:id` is called and the extraction's status is `failed`, the response shall additionally include a typed failure reason and a descriptive failure message that distinguishes the kind of failure from other failure kinds.
4. When `GET /api/extractions/:id` is called and the extraction's status is `pending` or `running`, the response shall not include the artifact fields (`title`, `body_markdown`, `metadata`) and shall not include a failure reason.
5. If `GET /api/extractions/:id` is called with an identifier that is not present in the catalogue, the document-extraction service shall return an HTTP 404 response.
6. If the request omits or presents an invalid `X-API-Token`, the document-extraction service shall return an HTTP 401 response and shall not look up the extraction.
7. The artifact fields exposed by `GET /api/extractions/:id` shall be identical to the values recorded at extraction time; no field is re-derived or mutated on read.

### Requirement 3: Markdown normalization output

**Objective:** As the researcher (and the LLM consumer downstream), I want every extracted artifact to follow the same normalization rules so that the output is structurally predictable and math-faithful regardless of which extraction tool produced it.

#### Acceptance Criteria

1. The document-extraction service shall emit inline math expressions wrapped in `$...$` and display math expressions wrapped in `$$...$$` regardless of the delimiter convention used by the underlying extraction tool.
2. The document-extraction service shall preserve markdown headings (e.g. `##`) and paragraph structure produced by the extraction tool.
3. When the extracted content contains a heading whose text matches `references`, `bibliography`, or `works cited` (case-insensitive, exact match on the heading line), the document-extraction service shall remove that heading and every line that follows it from the persisted body.
4. The document-extraction service shall not include tables, images, or figure captions in the persisted body, even when the underlying extractor emits them.
5. When the extracted content contains at least one `#` (level-1) heading, the document-extraction service shall use the text of the first `#` heading as the artifact's title.
6. When the extracted content contains no `#` heading, the document-extraction service shall use the source PDF's filename, with its directory and extension stripped, as the artifact's title.
7. The document-extraction service shall report a `word_count` equal to the count of whitespace-separated tokens in the persisted body.
8. The document-extraction service shall set the artifact's `metadata.content_type` to the value of `source_type` recorded at submission time.

### Requirement 4: Failure taxonomy and word-count gate

**Objective:** As the researcher and operator, I want every failure to carry a distinct, machine-readable reason and a descriptive message so that I can tell precisely why an extraction did not produce an artifact without parsing free-form text.

#### Acceptance Criteria

1. If the input PDF contains no extractable text (e.g. a scanned image-only PDF), the document-extraction service shall transition the extraction to status `failed` with reason `scanned_pdf` and a descriptive message naming the condition.
2. If the input PDF is corrupt or cannot be parsed by the extraction tool, the document-extraction service shall transition the extraction to status `failed` with reason `parse_failed` and a descriptive message.
3. If extraction fails for any reason not covered by `scanned_pdf` or `parse_failed` (for example, the configured extraction tool is unreachable, the file at `pdf_path` cannot be opened, or the tool itself terminates abnormally), the document-extraction service shall transition the extraction to status `failed` with reason `extractor_failure` and a descriptive message that surfaces the underlying error.
4. While the configured `max_words` threshold is `N` (default `50000`, operator-configurable at startup), when an extraction completes and its persisted body contains more than `N` whitespace-separated tokens, the document-extraction service shall transition the extraction to status `failed` with reason `too_large` and a descriptive message naming the actual word count and the configured threshold.
5. The document-extraction service shall ensure that `scanned_pdf`, `parse_failed`, `extractor_failure`, `too_large`, `expired`, and `process_restart` are surfaced as distinct, mutually exclusive failure reasons; any single extraction shall carry at most one terminal failure reason.

### Requirement 5: Job lifecycle, expiry, and restart recovery

**Objective:** As the operator, I want the lifecycle of an extraction to be observable, bounded in time, and resilient to process restarts, so that no extraction can sit indefinitely in a non-terminal state and a restart cannot leave a job stuck mid-run.

#### Acceptance Criteria

1. The document-extraction service shall progress every extraction through the states `pending → running → done | failed`, with `done` and `failed` as terminal states.
2. While the configured `job_expiry` is `T` (default `1h`, operator-configurable at startup), when the document-extraction service begins processing an extraction whose creation timestamp is older than `T`, it shall transition the extraction to `failed` with reason `expired` without invoking the extraction tool.
3. The document-extraction service shall apply the expiry check both to extractions that have been waiting in `pending` and to extractions that were in `running` when the previous process exited; both paths shall produce the same `expired` outcome when the threshold is exceeded.
4. While an extraction is in `pending` status and its creation timestamp is older than `job_expiry`, the document-extraction service shall not transition the extraction to `expired` until the moment processing would otherwise begin; the service shall not run any scheduled sweep that transitions extractions to `expired` ahead of pickup.
5. When the document-extraction service starts up, by the time it serves its first request, no extraction shall remain in `running` status; every extraction whose persisted status was `running` at the prior process exit shall instead be in `failed` with reason `process_restart`.
6. When the document-extraction service starts up, it shall pick up every extraction whose persisted status is `pending` and drive each to a terminal state without requiring any operator action or any new HTTP request.
7. When `POST /api/extractions` is called for a `(source_type, source_id)` whose prior extraction is in a terminal state (`done` or `failed`), the document-extraction service shall accept the request as a re-extraction per Requirement 1, AC 6.

### Requirement 6: Pluggable extractor, storage invariant, and startup readiness

**Objective:** As the operator, I want the underlying extraction tool to be replaceable without changing the system's behavior contract, the catalogue to enforce the one-artifact-per-paper invariant at the storage layer, and the service to refuse to start when its storage surface is not ready, so that tool migrations are clean and a corrupted or silent-broken-startup state is impossible.

#### Acceptance Criteria

1. The document-extraction service shall depend on a single extraction-tool contract; the choice of which tool fulfils that contract shall be made at startup. Replacing the tool shall not change the request, response, status, failure-reason, or normalization behavior defined by Requirements 1 through 5.
2. The document-extraction service shall ship MinerU as the v1 extractor implementation behind that contract.
3. The document-extraction service shall guarantee that at most one artifact exists for any given `(source_type, source_id)` pair in the catalogue, regardless of how `POST /api/extractions` requests arrive (serialised, concurrent, or through any internal path that reaches the storage).
4. If two distinct `POST /api/extractions` requests for the same `(source_type, source_id)` are received concurrently, the document-extraction service shall accept exactly one as the prevailing artifact and treat the other per Requirement 1, AC 6 (overwrite-in-place); at no intermediate point shall two artifacts with the same pair be visible to a `GET /api/extractions/:id` request.
5. The document-extraction service shall be ready to serve `POST /api/extractions` and `GET /api/extractions/:id` requests as soon as it begins accepting HTTP traffic; no request shall be rejected because the catalogue's storage was not yet initialised.
6. If the document-extraction service cannot initialise the catalogue's storage during startup, it shall fail to start and report the failure rather than accepting any request.
