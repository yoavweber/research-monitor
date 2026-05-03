# Requirements Document

## Introduction

The DeFi research-monitor backend currently fetches paper metadata (including a PDF URL on each `Paper`) but no component owns materializing the PDF bytes locally for downstream tools. Document extraction needs a real on-disk path to a PDF (its tool requires `-p <path>`), and that path is supplied directly by the HTTP caller today — there is no production path from a paper's PDF URL to a stored PDF on disk.

This feature introduces the **PDF Store**: a single capability that, given a stable paper identity and its PDF URL, ensures the PDF bytes are present in a known location and hands the caller back a handle for reading them. The handle exposes both a real on-disk path (so today's path-only consumer keeps working) and a stream-style read operation (so a future remote-storage backend can plug in without changing any caller). Behavior is idempotent on repeat calls, atomic on writes, and keep-forever in v1.

## Boundary Context

- **In scope**:
  - Materializing a PDF locally given a stable identity (`source_type`, `source_id`) and a URL.
  - Idempotent behavior on repeated calls for the same identity.
  - Atomic visibility (no consumer ever observes a partial file).
  - Operator-configurable storage root.
  - Surfacing fetch and write failures with enough information that the caller can decide retry vs. give-up.
  - A handle abstraction that callers depend on, separating "where the bytes live" from "how I read them".
- **Out of scope**:
  - Any change to `arxiv-fetcher`, `paper-persistence`, or `document-extraction` (integration of the PDF Store into the extraction flow is a follow-on update to `document-extraction`).
  - HTTP endpoints for browsing, downloading, or mutating stored PDFs.
  - Background sweepers, retention windows, TTL, or content-hash deduplication.
  - Non-local backends (object storage, NFS, etc.) and any orchestration that uses them.
  - Authorization, rate limiting, or quota enforcement on PDF retrieval.
- **Adjacent expectations**:
  - Callers supply a stable `(source_type, source_id)` identity and the URL to fetch from. The PDF Store does not derive identity from a `Paper` model; mapping is the caller's job.
  - The HTTP fetching capability already exists in the codebase as a generic byte-level GET. The PDF Store consumes it; it does not re-implement HTTP.
  - Operator configures the storage root via environment configuration, with a sensible default.

## Requirements

### Requirement 1: Materialize PDF on demand

**Objective:** As a downstream consumer (e.g., the document-extraction worker), I want to ask the PDF Store for a paper's PDF and receive a handle to its bytes, so that I can run path-based or stream-based tools without managing downloads, file paths, or temp files myself.

#### Acceptance Criteria
1. When a caller requests a PDF with a valid `(source_type, source_id, url)` and no PDF is currently stored for that identity, the PDF Store shall fetch the PDF bytes from the URL, persist them under the identity, and return a handle that resolves to the persisted bytes.
2. When a caller requests a PDF with a valid `(source_type, source_id, url)` and a PDF is already stored for that identity, the PDF Store shall return a handle to the existing stored bytes without fetching the URL again.
3. The PDF Store shall return a handle that exposes both a real on-disk path and a stream-based read operation, so that callers needing either consumption mode are served by the same return value.
4. If `source_type`, `source_id`, or `url` is empty or otherwise malformed, the PDF Store shall reject the request with a validation error and shall not perform any fetch or write.
5. While a fetch is in progress for a given identity, the PDF Store shall ensure that no caller observes a partially-written file under the canonical location for that identity.

### Requirement 2: Idempotency and atomic visibility

**Objective:** As an operator, I want repeat requests for the same paper and concurrent requests across the system to be safe, so that I never see duplicate downloads, corrupted files, or "sometimes works" extraction failures.

#### Acceptance Criteria
1. When the same `(source_type, source_id)` identity is requested two or more times, the PDF Store shall produce the same stored bytes for every successful return and shall fetch the URL at most once for the cached lifetime of those bytes.
2. The PDF Store shall publish stored bytes under their canonical location only after the bytes have been fully written, so that any handle returned by a successful call resolves to a complete file.
3. If a prior attempt left a partial or temporary artifact in the storage area (for example, after a crash mid-write), the PDF Store shall recover on the next request for that identity by overwriting the partial artifact and producing a complete file before returning.
4. When the stored file for an identity exists but is empty, the PDF Store shall treat it as not stored and shall fetch the URL to replace it.

### Requirement 3: Failure surfacing

**Objective:** As a downstream consumer, I want fetch and storage failures to be reported with enough specificity that I can distinguish a transient network error from a permanent input error, so that retry policy lives in the caller, not buried inside the store.

#### Acceptance Criteria
1. If the URL fetch fails (transport error, non-success HTTP status, or context cancellation), the PDF Store shall return an error that preserves the underlying cause and shall not leave any file under the canonical location for that identity.
2. If writing to the storage area fails (for example, the storage root is missing, not writable, or out of space), the PDF Store shall return an error that identifies the failure as a storage failure rather than a fetch failure, and shall not leave any partial file under the canonical location for that identity.
3. When the caller's context is cancelled mid-fetch or mid-write, the PDF Store shall abandon the operation, surface the cancellation to the caller, and shall not leave any partial file under the canonical location for that identity.
4. The PDF Store shall not silently substitute an empty or placeholder file when a fetch or write fails.

### Requirement 4: Handle abstraction for future backends

**Objective:** As the system maintainer, I want callers to depend on a handle abstraction rather than on "a path on this machine", so that switching to a remote storage backend later does not require changing any caller.

#### Acceptance Criteria
1. The PDF Store shall return its result through a handle abstraction that decouples "where the bytes live" from "how the caller reads them".
2. The handle shall provide a real on-disk path that callers requiring a path argument (for example, command-line tools) can use to read the PDF.
3. The handle shall provide a stream-based read operation that callers preferring streaming consumption can use without knowing the underlying storage location.
4. Where a future non-local backend is introduced, the PDF Store shall continue to satisfy both handle accessors so that no caller needs to change to support it.

### Requirement 5: Configurable storage root

**Objective:** As an operator, I want to choose where stored PDFs live on disk, so that I can place them on the appropriate volume for my deployment without code changes.

#### Acceptance Criteria
1. The PDF Store shall read its storage root location from environment configuration at startup and shall use a project-default location when the environment value is unset.
2. When the configured storage root does not exist at startup, the PDF Store shall create it before serving any request.
3. If the configured storage root exists but is not a directory or is not writable, the PDF Store shall fail at startup with an error that identifies the misconfigured root, rather than failing later on the first request.
4. The PDF Store shall organize stored files under the storage root in a layout keyed by `source_type` and `source_id`, so that an operator can locate the file for a given paper by inspection.

### Requirement 6: Retention

**Objective:** As an operator, I want stored PDFs to remain available across restarts and across re-extraction attempts, so that I do not pay re-download cost for work the system has already done.

#### Acceptance Criteria
1. The PDF Store shall retain stored PDFs indefinitely; it shall not delete, rotate, or evict stored files on its own.
2. When the system restarts, the PDF Store shall continue to serve previously-stored PDFs from the existing storage root without re-fetching them.
3. Where a future retention policy is introduced, it shall be opt-in and shall not change the default keep-forever behavior of v1.

### Requirement 7: Observability

**Objective:** As an operator, I want to see when PDFs are fetched, served from storage, or fail, so that I can diagnose extraction problems and understand network/disk usage.

#### Acceptance Criteria
1. When the PDF Store fetches a URL because the identity was not stored, it shall emit a structured log entry that identifies the identity and the outcome (success or failure category).
2. When the PDF Store serves a request from already-stored bytes without fetching, it shall emit a structured log entry that identifies the identity and indicates the cache-hit outcome.
3. If the PDF Store returns a fetch or storage error, it shall emit a structured log entry at warn or error level that identifies the identity and the failure category.
4. The PDF Store shall not log the PDF bytes themselves or any portion of the response body.
