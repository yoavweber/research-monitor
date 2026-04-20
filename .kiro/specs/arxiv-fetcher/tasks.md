# Implementation Plan

- [ ] 1. Foundation — paper domain aggregate and shared port update
- [x] 1.1 Define the paper domain aggregate
  - Introduce `Entry` and `Query` value objects with the fields specified in design.md (source-neutral; `SourceID`, `Version`, `Categories`, `MaxResults`, etc.).
  - Define the `UseCase` and `Fetcher` ports.
  - Define the three upstream-failure sentinels (`ErrUpstreamBadStatus`, `ErrUpstreamMalformed`, `ErrUpstreamUnavailable`) as `*shared.HTTPError` values following the existing `source.ErrNotFound` pattern.
  - `go build ./internal/domain/paper/...` compiles cleanly and the new types are importable from other layers.
  - _Requirements: 1.3, 2.3, 2.4, 3.2, 4.1, 4.2, 4.3_

- [x] 1.2 Rename the shared fetcher port and add the transport sentinel
  - Rename the pre-existing `shared.APIFetcher` to `shared.Fetcher` and change its signature so it takes a URL string and returns `([]byte, error)`.
  - Update the port's doc comment to describe the generic byte-GET contract (URL in, bytes out; non-2xx wraps `shared.ErrBadStatus`; transport failures bubble up stdlib errors).
  - Add `shared.ErrBadStatus` as a sentinel `error` value in the shared errors file.
  - `go build ./...` succeeds and no stale `APIFetcher` reference remains in the tree.
  - _Requirements: 1.1, 4.1_

- [ ] 2. Foundation — environment configuration
- [ ] 2.1 Add arxiv environment variables with fail-fast validation
  - Extend the bootstrap `Env` struct with `ARXIV_BASE_URL`, `ARXIV_CATEGORIES` (raw CSV + post-parse `[]string`), and `ARXIV_MAX_RESULTS`.
  - Teach `LoadEnv` to split the CSV, trim each element, drop empties, and fail with a descriptive error when the resulting list is empty.
  - Validate `ARXIV_MAX_RESULTS` lies in `[1, 30000]` and surface a clear error otherwise.
  - Document the three new variables in `.env.example` with sensible placeholder values.
  - `LoadEnv` returns an error on missing, empty, whitespace-only, zero, negative, non-numeric, or `> 30000` inputs, and returns populated `ArxivCategories` on valid input.
  - _Requirements: 2.1, 2.2, 3.3_

- [ ] 3. Core — generic byte fetcher
- [ ] 3.1 (P) Implement the generic byte fetcher
  - Provide a constructor that takes a request timeout and a `User-Agent` string and returns a `shared.Fetcher`.
  - Perform a single `GET` per call with the configured `User-Agent` and a permissive `Accept` header; do not retry or follow unusual redirect policies.
  - On 2xx, return the response body. On non-2xx, return an error wrapping `shared.ErrBadStatus` together with the received status code. On a read-body failure after headers, return an error the caller can classify as "no complete response".
  - Cover 200 success, non-2xx (identifies `shared.ErrBadStatus` via `errors.Is`), and handler-sleep beyond the timeout (identifies `context.DeadlineExceeded`) with `httptest.Server`-backed tests.
  - `go test ./internal/infrastructure/http/...` passes end-to-end.
  - _Requirements: 4.1, 4.3_
  - _Boundary: infrastructure/http_

- [ ] 4. Core — arxiv adapter (parser + fetcher)
- [ ] 4.1 (P) Implement the Atom parser with fixture tests
  - Provide a pure function that decodes an arXiv Atom feed into `[]paper.Entry` and returns `paper.ErrUpstreamMalformed` on any decode failure or on detected Atom-wrapped error entries (`<id>http://arxiv.org/api/errors#…</id>`).
  - Populate `SourceID`/`Version` by stripping `http://arxiv.org/abs/` from `<id>` and splitting the trailing `vN` suffix; pull `PDFURL` from the `<link title="pdf">` element rather than constructing it.
  - Add four fixture files under the package's `testdata/` directory: happy multi-entry, valid-empty, malformed XML, and Atom-wrapped error entry.
  - Treat a valid feed with zero entries as success (returns `([]paper.Entry{}, nil)`), not as an error.
  - All four fixture-driven test cases assert the exact expected outcomes and the parser test file runs cleanly.
  - _Requirements: 1.3, 1.5, 4.2_
  - _Boundary: infrastructure/arxiv (parser)_

- [ ] 4.2 Implement the arxiv fetcher composite
  - Provide a constructor that takes a base URL and a `shared.Fetcher` and returns a `paper.Fetcher`.
  - Build the arxiv-specific query string from `paper.Query`: OR across categories (`cat:X+OR+cat:Y`, parentheses only when there is more than one category), fixed `sortBy=submittedDate&sortOrder=descending`, and `max_results` from the Query. Assemble the URL via `net/url` so characters are encoded safely.
  - Delegate the GET to the injected `shared.Fetcher`. Translate `shared.ErrBadStatus` to `paper.ErrUpstreamBadStatus`; translate `context.DeadlineExceeded`, `net.Error.Timeout() == true`, `*url.Error` wrapping a network-layer failure, and read-body failures to `paper.ErrUpstreamUnavailable`. Any unclassified transport error falls back to `paper.ErrUpstreamUnavailable`.
  - On successful bytes, call the parser and propagate its result (including `paper.ErrUpstreamMalformed`) without modification. Never return a non-empty entries slice alongside a non-nil error.
  - Unit tests drive the fetcher with an inline fake `shared.Fetcher` that captures the URL and injects each error type; assertions cover single vs. multi-category URL shape, all three sentinel translations, and successful pass-through of parser output.
  - _Requirements: 2.3, 2.4, 3.1, 3.2, 3.4, 4.1, 4.3, 4.4_

- [ ] 5. Core — application use case
- [ ] 5.1 (P) Implement the arxivUseCase orchestrator
  - Provide a constructor that takes a `paper.Fetcher`, a `shared.Logger`, and an immutable `paper.Query`, and returns a `paper.UseCase`.
  - On invocation, make exactly one call to the fetcher, log exactly one structured outcome line per call, and return whatever the fetcher returns (never re-wrap or re-classify sentinels).
  - Success log: `InfoContext("paper.fetch.ok", "source", "arxiv", "count", N, "categories", [...])`. Failure log: `WarnContext("paper.fetch.failed", "source", "arxiv", "category", "bad_status"|"malformed"|"unavailable", "err", err)`.
  - Do not inspect HTTP status codes, raw bytes, XML, or `net/http` error types at any point — the only vocabulary the use case recognizes is the three `paper.*` sentinels.
  - Unit tests with an inline fake `paper.Fetcher` cover success (entries returned, `fetch.ok` log emitted once, category count correct), each of the three sentinels (each produces exactly one `fetch.failed` log with the matching `category` field), and return-value identity (`errors.Is` against the injected sentinel passes).
  - _Requirements: 1.1, 1.4, 1.5, 3.4, 4.4, 4.5_
  - _Boundary: application/arxiv_

- [ ] 6. Core — HTTP layer
- [ ] 6.1 (P) Implement the arxiv controller and response DTOs
  - Define `FetchResponse`, `EntryResponse`, and the `ToFetchResponse(entries, fetchedAt)` mapper inside the arxiv controller package; the domain layer must not carry any response DTOs.
  - Implement the `Fetch` handler to call `paper.UseCase.Fetch(c.Request.Context())`, pass errors to `c.Error(err)` without status mapping, and return `http.StatusOK` with `common.Data(ToFetchResponse(...))` on success.
  - Handler accepts no body and no query parameters; it inherits auth from the enclosing `/api` group.
  - Unit tests with a fake `paper.UseCase` cover: success body JSON shape (fields match the wire schema), empty success (`data.entries == []`, `data.count == 0`), and each `paper.*` sentinel (controller forwards to `c.Error`, the error envelope middleware renders the expected status).
  - _Requirements: 1.1, 1.3, 1.5, 4.4_
  - _Boundary: interface/http/controller/arxiv_

- [ ] 6.2 Extend route.Deps and register the arxiv router
  - Add an `Arxiv ArxivConfig{Fetcher paper.Fetcher, Query paper.Query}` field to `route.Deps`.
  - Implement `ArxivRouter(d Deps)` that constructs the use case and controller locally from `d.Arxiv` + `d.Logger` and registers `GET /arxiv/fetch` on `d.Group`.
  - Wire `ArxivRouter(d)` into `route.Setup` after the existing `SourceRouter(d)` call.
  - `go build ./internal/interface/http/...` succeeds and the new endpoint is registered under the `/api` group (which already mounts the `APIToken` middleware, giving requirement 1.2 for free).
  - _Requirements: 1.1, 1.2_

- [ ] 7. Integration — bootstrap wiring and test harness
- [ ] 7.1 Compose the runtime pipeline in app bootstrap
  - In `bootstrap/app.go`, construct the generic byte fetcher with a fixed timeout and a descriptive `User-Agent` that includes a contact URL.
  - Wrap the byte fetcher in the arxiv fetcher using `env.ArxivBaseURL`, assemble the immutable `paper.Query` from `env.ArxivCategories` and `env.ArxivMaxResults`, and pass both to `route.Setup` via `route.Deps.Arxiv`.
  - Starting the service with a valid `.env` and issuing `GET /api/arxiv/fetch` with a valid `X-API-Token` returns a 200 response whose body is shaped like `FetchResponse` (with either real arxiv entries or an empty list, depending on upstream).
  - _Requirements: 1.1, 2.3, 3.1, 3.2_

- [ ] 7.2 Extend the integration test harness
  - Teach `tests/integration/setup.SetupTestEnv` to accept an option that injects a fake `paper.Fetcher` and a fixed `paper.Query`, and to register `ArxivRouter` inside the same `/api` group that mounts the test `APIToken` middleware.
  - Add a hand-written `paper.Fetcher` fake in `tests/mocks/` that (a) records every `paper.Query` it receives, (b) returns a caller-configured `[]paper.Entry` or any of the three `paper.*` sentinels, and (c) counts invocations so tests can assert zero calls.
  - Calling the returned test server's `/api/arxiv/fetch` with the harness test token lands in the fake (observable by reading the fake's recorded-invocation counter from the test).
  - _Requirements: 1.2_

- [ ] 8. Validation — endpoint integration tests
- [ ] 8.1 Exercise the endpoint end-to-end through the fake paper.Fetcher
  - 200 happy: fake returns canned entries → response body matches the wire schema and the fake's recorded `paper.Query` equals the harness-configured one.
  - 200 empty: fake returns `([]paper.Entry{}, nil)` → `data.entries` is `[]` and `data.count` is `0`.
  - 401: request sent without (and with an invalid) `X-API-Token` returns 401, and the fake's invocation counter remains at zero.
  - 502 (bad status): fake returns `paper.ErrUpstreamBadStatus` → response is a 502 with the standard error envelope.
  - 502 (malformed): fake returns `paper.ErrUpstreamMalformed` → response is a 502 with the standard error envelope.
  - 504: fake returns `paper.ErrUpstreamUnavailable` → response is a 504 with the standard error envelope.
  - The integration test file runs cleanly under the existing `integration` build tag, alongside the pre-existing source-aggregate integration tests.
  - _Requirements: 1.1, 1.2, 1.5, 4.1, 4.2, 4.3, 4.5_
  - _Depends: 7.1, 7.2_
