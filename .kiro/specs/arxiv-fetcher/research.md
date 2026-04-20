# Research & Design Decisions — arxiv-fetcher

## Summary

- **Feature**: `arxiv-fetcher`
- **Discovery Scope**: Extension (first concrete impl of a pre-existing domain port inside an established hexagonal backend)
- **Key Findings**:
  - The `APIFetcher` port (`internal/domain/shared/ports.go`) is generic and byte-level (`Fetch(ctx, endpoint) ([]byte, error)`), so it accommodates arXiv's Atom/XML response without changes.
  - All reusable infrastructure for this feature already exists: `APIToken` middleware, `ErrorEnvelope` middleware, `HTTPError` sentinel pattern (see `source.ErrNotFound`), `route.Deps` wiring pattern, and viper-backed env loading with fail-fast validation.
  - arXiv's public API has quirks that must be designed for up front: errors are returned as HTTP 200 with an Atom `<entry>` whose `<title>` is "Error", rate-limit etiquette is 1 request / 3 seconds, and HTTPS is supported but documented as HTTP. Rolling our own `net/http` + `encoding/xml` client is the conventional approach — none of the community Go libraries are production-grade.

## Research Log

### arXiv HTTP API contract

- **Context**: Requirements 3.1–3.4 and 4.1–4.5 depend on concrete knowledge of the arXiv query API, including failure modes and the exact Atom shape.
- **Sources Consulted**:
  - [arXiv API User Manual](https://info.arxiv.org/help/api/user-manual.html)
  - [arXiv API Basics](https://info.arxiv.org/help/api/basics.html)
  - [arXiv API Terms of Use](https://info.arxiv.org/help/api/tou.html)
- **Findings**:
  - Canonical endpoint: `https://export.arxiv.org/api/query` (HTTPS works and is preferred).
  - Multi-category OR query: `search_query=cat:q-fin.ST+OR+cat:cs.LG` (operators uppercase; `+` for spaces; parentheses URL-encoded as `%28`/`%29` when grouping).
  - Sort: `sortBy=submittedDate&sortOrder=descending`.
  - Pagination: `start` (0-based offset) and `max_results` (hard cap 30000; values >30000 → HTTP 400). Per-call 100–500 is the typical polite default; 2000 per call is the manual's suggestion for bulk harvesting.
  - Atom entry shape: `<id>` → `http://arxiv.org/abs/{id}[v{n}]`; `<title>`; multiple `<author><name>` children; `<summary>` (abstract); `<arxiv:primary_category term="cs.LG"/>` plus sibling plain `<category term="…"/>` entries; `<published>` = first-submission date (use this for "submission date"); `<updated>` = latest-version date; `<link title="pdf" rel="related" type="application/pdf" href="…"/>` present on every entry (parse it, do not hand-construct).
  - Error behavior: **arXiv returns HTTP 200** with an Atom feed containing a single `<entry>` whose `<title>` is "Error" and whose `<summary>` carries the message; `<id>` points to `http://arxiv.org/api/errors#…`. HTTP 400 is reserved for gross misuse (e.g., `max_results` > 30000). Downtime surfaces as 5xx from the upstream frontend.
  - Content-Type: `application/atom+xml; charset=utf-8` for both success and error bodies.
  - Rate limit: 1 request / 3 seconds, single connection at a time — Terms of Use, cross-machine.
  - User-Agent: no mandated format. Convention is descriptive UA with contact email, e.g., `defi-monitor/1.0 (mailto:you@example.com)`.
- **Implications**:
  - The client must detect the "Error" entry and treat it the same as a malformed response — requirement 4.2 maps to both raw XML parse failure and the Atom-wrapped error entry. This behavior is captured in the parser and surfaced as `ErrUpstreamMalformed` (502).
  - Non-2xx responses from arXiv map to `ErrUpstreamBadStatus` (502, requirement 4.1).
  - Network failures and request timeouts both map to `ErrUpstreamUnavailable` (504, requirement 4.3).
  - Rate-limit etiquette is out of scope for v1 (manual-trigger only, single concurrent caller expected); noted as a follow-up when a scheduler is added.

### Codebase extension points

- **Context**: Where the new code sits in the existing hexagonal backend, and what must be modified vs. added.
- **Sources Consulted**: Direct reads of `internal/domain/shared/`, `internal/interface/http/route/`, `internal/interface/http/middleware/`, `internal/interface/http/common/`, `internal/bootstrap/`, and the existing `source` aggregate as a pattern reference.
- **Findings**:
  - `APIToken` middleware already exists (`interface/http/middleware/api_token.go`) and is already applied at the `/api` group level in `bootstrap/app.go`. No new auth code is required; the new endpoint inherits auth by registering under the `/api` group's `d.Group`.
  - `ErrorEnvelope` middleware already translates `*shared.HTTPError` into the standard error envelope. Domain-layer error sentinels following the `source.ErrNotFound = shared.NewHTTPError(…)` pattern are the idiomatic way to encode the 502/504 distinction required by requirement 4.
  - `route.Deps` currently carries `Group`, `DB`, `Logger`, `Clock` only. Feature-scoped dependencies (the arxiv fetcher impl + parsed category list + max results) need a dedicated sub-bundle to avoid cluttering `Deps` as more feature deps are added.
  - Viper env loading in `bootstrap/env.go` already enforces fail-fast validation (`API_TOKEN` and `SQLITE_PATH` are required). Extending this pattern for `ARXIV_*` keeps configuration discipline consistent and directly satisfies requirements 2.2 and 3.3.
  - `tests/mocks/` is empty; a hand-written `shared.APIFetcher` fake must be authored (the steering explicitly forbids mock-generation tools).
  - `tests/integration/setup/setup.go` currently wires the `source` routes only. It must be extended to accept an injected fake `shared.APIFetcher` and register the arxiv router against it.
- **Implications**:
  - No new middleware, no new cross-cutting infrastructure — purely additive inside the established layer rules.
  - The stale comment on `shared.APIFetcher` ("for JSON API ingestion sources") is touched to correct "JSON" → "byte-level" since arXiv returns XML. Documentation-only.

### Atom parsing approach

- **Context**: Decide whether to adopt a Go library or roll our own parser.
- **Sources Consulted**: pkg.go.dev searches for `arxiv` Go clients; Atom spec via arXiv manual examples.
- **Findings**: Three community Go arxiv libraries exist (`orijtech/arxiv`, `Epistemic-Technology/arxiv`, `marvin-hansen/arxiv`). None is production-grade — low stars, low commit frequency, no CVE disclosure process, and all pull in additional transitive dependencies. The Atom schema used by arXiv is small and stable, and `encoding/xml` handles namespaces correctly with `xml:"http://arxiv.org/schemas/atom primary_category"` tags.
- **Implications**: Build, don't adopt. A ~100-line pure function in the application layer parses the Atom feed to `[]arxiv.Entry`, with fixture-driven unit tests covering happy, empty, malformed, and Atom-wrapped-error cases.

## Architecture Pattern Evaluation

| Option | Description | Strengths | Risks / Limitations | Notes |
|--------|-------------|-----------|---------------------|-------|
| Port + single-pass use case | `shared.APIFetcher` returns bytes; use case builds URL, parses XML, returns domain entries. | Keeps the generic byte port stable; parser is a pure function, trivially testable. | Use case holds URL-building and parser orchestration — two responsibilities. | Selected. |
| arxiv-specific port | Define `arxiv.Client` returning `[]Entry` directly; bypass the generic port. | Single cohesive seam per feature. | Contradicts requirement's explicit ask ("concrete arXiv implementation of the `APIFetcher` port"); blocks future reuse of the generic byte port for governance forums. | Rejected. |
| Library-based client | Adopt a community Go arxiv library. | Less code. | No library is production-grade; transitive deps; less control over error mapping (504 vs 502 split). | Rejected. |

## Design Decisions

### Decision: Implement `shared.APIFetcher` as an arxiv-preconfigured HTTP byte client

- **Context**: Requirements frame the deliverable as "a concrete arXiv implementation of the `APIFetcher` port". The port is byte-level and generic.
- **Alternatives Considered**:
  1. Introduce a new `arxiv.Client` domain port returning `[]Entry` — rejected (contradicts the requirement and blocks reuse of the generic port for the next API source).
  2. Build a completely generic HTTP byte fetcher in `infrastructure/http/` — rejected as over-abstraction; no other caller exists today and the arxiv-specific defaults (base URL, User-Agent, Accept header, timeout) are what give the port impl identity.
- **Selected Approach**: A concrete `shared.APIFetcher` implementation lives at `internal/infrastructure/arxiv/client.go`. The constructor accepts base URL, User-Agent, and timeout and returns the generic `shared.APIFetcher` interface. The use case passes the query string as the `endpoint` argument; the client joins it to the configured base URL.
- **Rationale**: Satisfies the requirement literally, respects the dependency direction, and keeps the generic port intact for future API sources.
- **Trade-offs**: The implementation is de facto single-purpose for now, but that is accurate: it is arxiv-tuned defaults over an otherwise generic byte GET. A second API source would either get its own preconfigured impl or motivate extracting a shared `infrastructure/http/byte_client.go` later.
- **Follow-up**: Revisit when the second API source (governance forums) is specced.

### Decision: XML parsing lives in the application layer as a pure function

- **Context**: Where does Atom parsing belong?
- **Alternatives Considered**:
  1. Port + infrastructure adapter (`arxiv.FeedParser`) — rejected as speculative abstraction; only one parsing implementation is ever foreseeable.
  2. Parse inside the infrastructure client — rejected because it would require the client to return domain types, breaking the generic byte port.
- **Selected Approach**: `internal/application/arxiv_parser.go` exposes an unexported `parseFeed([]byte) ([]arxiv.Entry, error)` used by the use case. The parser is fixture-tested.
- **Rationale**: Pure function, zero I/O, trivial to unit-test, no artificial seam.
- **Trade-offs**: Parser cannot be swapped out behind an interface at runtime. Acceptable — the Atom schema is stable.
- **Follow-up**: If a second ingestion source later needs structurally identical feed parsing, consider extracting a `domain/feed/` package.

### Decision: Encode HTTP failure categories via typed sentinel `*shared.HTTPError` values

- **Context**: Requirements 4.1–4.3 demand distinguishable failure signals (502 vs 504). The controller already delegates HTTP status selection to the `ErrorEnvelope` middleware, which reads `*shared.HTTPError`.
- **Alternatives Considered**:
  1. Map errors to status codes inside the controller — rejected; duplicates logic the middleware already handles and diverges from the `source` pattern.
  2. Use Go error-wrapping with a discriminated `errors.Is` check in the controller — rejected for the same reason.
- **Selected Approach**: Define three sentinels in `domain/arxiv/errors.go`:
  - `ErrUpstreamBadStatus` — `shared.NewHTTPError(502, …)`
  - `ErrUpstreamMalformed` — `shared.NewHTTPError(502, …)` (includes Atom-wrapped error entries per arXiv contract)
  - `ErrUpstreamUnavailable` — `shared.NewHTTPError(504, …)`
- **Rationale**: Idiomatic, matches `source.ErrNotFound`, and lets the existing middleware do the status mapping.
- **Trade-offs**: The sentinels embed their HTTP semantics, coupling the domain package to HTTP. This coupling already exists in the codebase (`source.ErrConflict` etc.) and is the house pattern.
- **Follow-up**: None.

### Decision: Endpoint is `GET /api/arxiv/fetch` (no query parameters)

- **Context**: Requirement 1.1 specifies "HTTP request" without pinning the method. Requirements 2.5 and 3.4 forbid runtime mutation of categories or page size. Requirement 3.5 specifies deterministic output across repeat calls with no upstream change.
- **Alternatives Considered**:
  1. `POST /api/arxiv/fetch` — defensible as "trigger an action", but POST implies a resource mutation or creation, which the server does not perform.
  2. `GET /api/arxiv/fetch?category=…&max_results=…` — rejected because it would leak runtime-mutation intent and contradict requirements 2.5 / 3.4.
- **Selected Approach**: `GET /api/arxiv/fetch`, no parameters, idempotent, side-effect-free from the server's perspective.
- **Rationale**: Semantically correct for a read-only operation, matches the determinism guarantee in requirement 3.5, and keeps the contract minimal.
- **Trade-offs**: GET may be cached by intermediaries. Acceptable because the response is intentionally "top of feed now" — a stale cached response is still a meaningful snapshot.
- **Follow-up**: Consider `Cache-Control: no-store` if a caching proxy is introduced.

### Decision: Feature-scoped dependency bundle on `route.Deps`

- **Context**: `route.Deps` currently carries only cross-cutting infra (DB, Logger, Clock). The arxiv router needs a `shared.APIFetcher` impl plus parsed `Categories []string` and `MaxResults int`. Adding these to `Deps` directly would dilute the shared bundle.
- **Alternatives Considered**:
  1. Add `Fetcher`, `Categories`, `MaxResults` as top-level `Deps` fields — rejected because each future feature would do the same.
  2. Pass the whole `*bootstrap.Env` to `Deps` — rejected because `interface/` cannot import `bootstrap/` per the dependency rule.
- **Selected Approach**: Add a feature-scoped sub-struct `route.ArxivConfig{ Fetcher, Categories, MaxResults }` and a single `Arxiv ArxivConfig` field on `Deps`.
- **Rationale**: Keeps `Deps` readable, scopes the feature wiring, and does not violate the import rule (all types come from `domain/shared`).
- **Trade-offs**: One extra struct per feature. Negligible.
- **Follow-up**: Apply the same pattern for the next feature.

## Risks & Mitigations

- **Risk**: arXiv returns HTTP 200 with an Atom-wrapped error entry, which a naive parser will silently misclassify as "empty results" or a successful response with a single garbage entry. — **Mitigation**: Parser detects `<entry><title>Error</title>…<id>http://arxiv.org/api/errors…</id></entry>` and returns `ErrUpstreamMalformed`. Fixture test covers this exact case.
- **Risk**: `max_results` > 30000 triggers HTTP 400 from arXiv, which the design currently maps to `ErrUpstreamBadStatus` (502). A 502 for an operator-config error is misleading. — **Mitigation**: Env validation at startup caps `ARXIV_MAX_RESULTS` at 30000 and fails to start on values above that cap. Requirement 3.3 already demands startup validation for invalid values.
- **Risk**: Network timeout vs. TLS handshake failure vs. DNS failure all map to `ErrUpstreamUnavailable` (504), but they surface through different `net/http` errors. — **Mitigation**: The client maps anything that isn't a successful response receipt (including `context.DeadlineExceeded`, `net.OpError`, `url.Error`) to `ErrUpstreamUnavailable`. Unit tests cover each error type.
- **Risk**: The current `APIFetcher` doc comment says "JSON API ingestion sources", which is wrong for arXiv. Future readers will assume JSON. — **Mitigation**: Update the comment in the same change to say "byte-level API ingestion sources".
- **Risk**: Integration tests against the real arXiv API would be flaky and violate the 3s/req rate limit in CI. — **Mitigation**: Integration tests use the `shared.APIFetcher` fake from `tests/mocks/` exclusively. A manual smoke-test path against the real endpoint is deferred.

## References

- [arXiv API User Manual](https://info.arxiv.org/help/api/user-manual.html) — canonical endpoint, parameters, Atom schema.
- [arXiv API Basics](https://info.arxiv.org/help/api/basics.html) — `search_query` prefixes and operators.
- [arXiv API Terms of Use](https://info.arxiv.org/help/api/tou.html) — 1 req / 3s rate limit.
- [Go `encoding/xml`](https://pkg.go.dev/encoding/xml) — namespace handling for `arxiv:primary_category`.
- Existing in-repo patterns: `internal/domain/source/errors.go` (HTTPError sentinel usage), `internal/interface/http/middleware/error_envelope.go` (status mapping), `internal/bootstrap/env.go` (fail-fast viper loading).
