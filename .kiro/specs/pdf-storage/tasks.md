# Implementation Plan — pdf-storage

- [ ] 1. Foundation: configuration and package skeleton

- [x] 1.1 Extend env configuration with PDF_STORE_ROOT field, default, and startup validation
  - Add a new env field for the storage root with the operator-facing variable `PDF_STORE_ROOT`.
  - Set the default value `data/pdfs` via the existing viper default mechanism so the variable is optional.
  - Validate at startup: the configured path must be creatable; if it already exists, it must be a directory and writable. On violation, produce a startup error that names the offending field and the underlying cause.
  - Update `.env.example` with the new variable and a one-line comment.
  - Observable completion: starting the binary with the variable unset uses `data/pdfs`; starting with the variable pointing at a regular file fails fast with a startup error mentioning `PDF_STORE_ROOT`.
  - _Requirements: 5.1, 5.3_

- [x] 1.2 Create the pdf domain package skeleton with package doc and error sentinels
  - Create the new `pdf` domain package with a package-level doc that states purpose and the dependency rule (stdlib + `domain/shared` only).
  - Declare the three exported error sentinels for invalid-key, fetch-failure, and store-failure categories. Document that each is intended to be used with `errors.Is` and that implementations wrap the underlying cause through `%w`.
  - Observable completion: the new package compiles in isolation; `go build ./...` from the repo root succeeds; the three sentinels are exported and reachable from outside the package.
  - _Requirements: 1.4, 3.1, 3.2_

- [ ] 2. Domain: identity value object and ports

- [x] 2.1 (P) Implement the PDF identity value object with validation
  - Define the value object that carries source type, source identifier, and the fetch URL.
  - Implement `Validate` to reject any empty-after-trim field with `ErrInvalidKey` wrapping a descriptive cause.
  - Add defense-in-depth checks that reject identifier fields containing path-traversal or path-separator characters; document this rule in the field godoc.
  - Provide colocated unit tests covering: every individual empty/whitespace field, traversal characters, and a happy-path arxiv-style identifier. Tests use `t.Parallel()` and `t.Run` per the steering rule even for single-case tests.
  - Observable completion: `go test ./internal/domain/pdf/...` passes; every rejection branch in `Validate` has at least one failing-input test asserting `errors.Is(err, ErrInvalidKey)`.
  - _Requirements: 1.4, 5.4_
  - _Boundary: domain/pdf identity value object_

- [x] 2.2 (P) Define the Store and Locator port interfaces
  - Declare the store port with a single method that takes a context and a key and returns a locator or an error.
  - Declare the locator port with a path accessor returning a real on-disk path and a context-aware open accessor returning a stream-like read closer.
  - Document each method's pre/post conditions, the error contract (which sentinel each path returns), and the locator's stability guarantees over its lifetime.
  - Observable completion: ports compile; consumers can reference the package-qualified types `pdf.Store` and `pdf.Locator`; godoc on each method matches the contract documented in design.
  - _Requirements: 1.1, 1.2, 1.3, 4.1, 4.2, 4.3, 4.4_
  - _Boundary: domain/pdf ports_

- [ ] 3. Local filesystem implementation

- [x] 3.1 (P) Implement the local locator with path and open
  - Create the local locator type that holds the canonical path and exposes both the path accessor and a context-aware open that returns a read closer over the on-disk file.
  - The open accessor accepts a context for interface conformance with future remote backends; v1 may ignore it but must not introduce a race on file-handle close if the caller never reads.
  - Provide colocated unit tests under a real temp directory: write a known byte sequence, build the locator, assert the path accessor and the open accessor return byte-for-byte the same content.
  - Observable completion: `go test ./internal/infrastructure/pdf/local/...` reports the locator test passing; the locator type is unexported and is returned through the `pdf.Locator` interface only.
  - _Requirements: 1.3, 4.2, 4.3_
  - _Boundary: infrastructure/pdf/local locator_

- [ ] 3.2 (P) Implement the local store constructor with root validation and canonical-path computation
  - Implement an unexported store type and an exported constructor that takes the storage root, a fetcher port, and a logger port.
  - The constructor calls `MkdirAll` on the root with mode `0o755`; if the root resolves to a regular file or is not writable, the constructor returns an error that mentions the misconfigured root and identifies the failure as a storage failure.
  - Implement the canonical-path helper that yields `<root>/<source_type>/<source_id>.pdf` for a key, ensuring it stays within the root for any key that has already passed `Validate`.
  - Provide colocated unit tests for: happy-path construction under a temp dir, root pointing at a regular file (constructor fails), and the canonical-path helper for representative inputs.
  - Observable completion: constructor fails fast in the misconfigured cases; the canonical-path helper produces the documented layout under test inspection.
  - _Requirements: 5.2, 5.3, 5.4_
  - _Boundary: infrastructure/pdf/local store construction_

- [ ] 3.3 Implement Ensure end-to-end with cache gate, atomic fetch-and-write, and error classification
  - Validate the key first; on rejection return without touching the filesystem or the fetcher.
  - Existence gate: when the canonical file exists with non-zero size, return a locator immediately without invoking the fetcher; treat zero-byte files as not stored and proceed to fetch.
  - Fetch-and-write recipe: ensure the per-source-type subdirectory exists, create a sibling temp file with a randomized suffix in the same directory, write the fetched bytes, close the temp file, then rename it to the canonical path. The canonical path must never link to a partially-written file.
  - Wrap fetcher errors as the fetch-failure sentinel with `%w` chaining the underlying cause so callers can inspect transport, status-code, and cancellation errors via `errors.Is`. Wrap filesystem errors as the store-failure sentinel similarly. Treat an empty fetcher response body as a fetch failure rather than writing an empty file.
  - On any error path between temp creation and successful rename, remove the temp file. On context cancellation, abandon the operation, surface the cancellation chained inside the fetch-failure wrapping, and leave no canonical file.
  - Provide colocated unit tests under a real temp directory using an inline fake fetcher (recording invocations, configurable response and error). Cover: cache miss writes atomically with no leftover temp siblings and exactly one fetcher invocation; cache hit returns without invoking the fetcher; zero-byte canonical file replaced via fetch; fetcher status-code error surfaces both `ErrFetch` and `shared.ErrBadStatus`; filesystem write failure surfaces `ErrStore`; context cancellation mid-fetch surfaces `context.Canceled` chained under `ErrFetch` with no canonical file produced; deliberate keep-forever assertion that a successful Ensure never deletes any prior file.
  - Observable completion: all unit tests pass; the canonical layout is `<root>/<source_type>/<source_id>.pdf`; no test leaves stray `*.tmp` siblings; `errors.Is` chains hold for every failure category.
  - _Requirements: 1.1, 1.2, 1.5, 2.1, 2.2, 2.3, 2.4, 3.1, 3.2, 3.3, 3.4, 5.4, 6.1_
  - _Boundary: infrastructure/pdf/local store Ensure_

- [ ] 3.4 Add structured logging for fetched, cache-hit, and failed events
  - Emit a structured info-level event when a fetch occurs, with fields for source type, source identifier, byte count, and elapsed milliseconds.
  - Emit a structured info-level event on cache hits, with fields for source type, source identifier, and byte count.
  - Emit a structured warn-level event on fetch failures and an error-level event on storage failures, both with a category field that distinguishes fetch / store / invalid_key, and a formatted error string. No log field may carry response body bytes.
  - Use the shared `RecordingLogger` test double under colocated tests to drive each path and assert event names, levels, and field keys; assert that the body is never present in any captured field.
  - Observable completion: tests assert exactly one event per code path with correct level and field set; a grep over the log call sites confirms no body byte slice is ever passed to the logger.
  - _Requirements: 7.1, 7.2, 7.3, 7.4_
  - _Depends: 3.3_
  - _Boundary: infrastructure/pdf/local logging_

- [ ] 4. Integration

- [ ] 4.1 Wire the local store into the bootstrap composition root and Deps surface
  - Construct the local store once at startup using the existing byte fetcher, the existing logger, and the new `PDFStoreRoot` config field. Surface the constructor error so a misconfigured root prevents startup.
  - Add the store to the shared `route.Deps` struct so that the future document-extraction integration can pick it up without another bootstrap edit. Leave it unused by any current route in this spec.
  - Confirm the dependency direction: domain/pdf imports stdlib + domain/shared only; infrastructure/pdf/local imports domain/pdf + domain/shared + infrastructure/httpclient symbol set; bootstrap is the only place concretes are instantiated.
  - Observable completion: `go build ./...` and `go vet ./...` succeed; running the binary with the default config creates `data/pdfs/` if missing and serves traffic; the store is reachable through `route.Deps` (compile-time check via a small test that constructs `route.Deps` from the bootstrap's wiring path).
  - _Requirements: 5.1, 5.2, 5.3, 6.2_
  - _Depends: 3.2, 3.3_
  - _Boundary: bootstrap composition_

- [ ] 4.2 Add a cross-package integration test using the real byte fetcher against an httptest server
  - Wire the real `httpclient` byte fetcher against an `httptest.Server` that returns a small known PDF byte sequence on a happy path and a non-2xx status on a sad path.
  - Drive the local store under a real temp directory: assert atomic write of the served bytes, that a second Ensure on the same key does not re-hit the test server (cache-hit confirmation), and that a deadline-bound context surfaces a fetch failure chaining `context.DeadlineExceeded`.
  - Place the test colocated with the local store implementation; no build tag needed (the existing `httpclient` test follows the same pattern).
  - Observable completion: `go test ./internal/infrastructure/pdf/local/...` passes including the new integration test; the test server's hit counter records exactly one request across the cache-miss-then-cache-hit sequence.
  - _Requirements: 1.1, 1.2, 1.5, 2.1, 2.2, 3.1, 3.3_
  - _Depends: 3.3, 4.1_
  - _Boundary: infrastructure/pdf/local integration_

## Implementation Notes

- Pre-existing dirty file `internal/infrastructure/persistence/extraction/model.go` is unrelated user scratchpad work; selective staging excludes it from every pdf-storage commit. Future reviewer prompts should note this to avoid false-positive boundary rejections.
- Env-vs-constructor validation split: env-side validates Stat + writability probe via `os.CreateTemp` (no `MkdirAll`); store constructor (Task 3.2) owns `MkdirAll`. Tests for env validation use `t.Setenv` and therefore correctly omit `t.Parallel()` per testing.md's process-global carve-out.

## Deferred Requirements

- **Requirement 6.3** ("future retention is opt-in"): this is a forward-design constraint with no code to write in v1. The design's keep-forever default is established by the absence of any deletion logic (asserted in Task 3.3). Any future retention work will arrive in a separate spec and must preserve the v1 default by being opt-in.

