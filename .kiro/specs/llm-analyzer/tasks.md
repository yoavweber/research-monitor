# Implementation Plan

- [ ] 1. Foundation: cross-cutting infrastructure that the analyzer depends on
- [x] 1.1 Extend the shared HTTPError envelope with an optional machine-readable reason
  - Add an optional reason field on the shared HTTPError type and a small constructor helper that sets it without breaking existing call sites.
  - Update the error envelope middleware so that when the unwrapped HTTPError carries a non-empty reason, the rendered JSON envelope exposes it under error.details.reason; when reason is empty, the existing wire shape stays unchanged.
  - Add unit tests covering both branches: a sentinel without a reason produces the existing envelope, and a sentinel with a reason produces an envelope whose error.details.reason matches.
  - Done when running the unit tests against the modified middleware shows both branches passing and no other call site of the envelope changes behavior.
  - _Requirements: 5.4_
  - _Boundary: shared.HTTPError, middleware.ErrorEnvelope_

- [x] 1.2 Add the LLM_PROVIDER configuration switch with startup validation
  - Add a flat top-level field for the LLM provider to the env struct, default to fake, and bind the env var.
  - Reject unknown provider values at startup with a clear error; reserve the anthropic value but reject it for now with an explicit not-implemented-yet message until that adapter ships.
  - Add unit tests that cover: default resolves to fake, unknown values fail startup, the reserved anthropic value fails startup with the not-implemented message.
  - Done when the bootstrap env loader returns a fake provider by default and refuses to start with any other value except the reserved one.
  - _Requirements: 7.1, 7.4_
  - _Boundary: bootstrap.env_

- [x] 1.3 Add a hand-written LLM client double for use-case tests
  - Place the double under the project's tests/mocks/ directory next to existing port doubles, following the project's hand-written-fakes-only convention.
  - Make per-call behavior programmable so a single test can script the short, long, and thesis call outcomes independently — including transport errors and arbitrary response text for malformed-envelope tests.
  - Done when the double can be constructed in a test, accept a sequence of canned responses, and be asserted against (call count, observed prompt versions).
  - _Requirements: 6.1, 6.3, 7.2_
  - _Boundary: tests/mocks_

- [ ] 2. Core: analyzer domain, persistence, application, and fake provider

- [x] 2.1 Define the analyzer domain package: value type, ports, sentinel errors
  - Define the persisted analysis value type with the fields documented in the design's Domain Model section.
  - Define the inbound use-case port and the outbound repository port with context-first method signatures.
  - Define the sentinel errors (extraction-not-found, extraction-not-ready, llm-upstream, malformed-response, analysis-not-found, catalogue-unavailable) wrapping the shared HTTPError with the codes and reason strings from the design's sentinel map.
  - Add a sentinel-mapping unit test that asserts each sentinel's wrapped HTTP code and (where applicable) its reason string match the design's table.
  - Done when the package compiles, exposes only the documented surface, and the sentinel-mapping test passes.
  - _Requirements: 1.1, 1.2, 1.3, 2.1, 2.2, 4.1, 4.2, 5.1, 5.2, 5.4, 5.5, 6.2_
  - _Boundary: domain/analyzer_

- [x] 2.2 (P) Implement the analyzer use case: prompt orchestration, JSON envelope parsing, fail-fast error mapping
  - Hold the three prompt strings and prompt-version constants (short.v1, long.v1, thesis.v1) together with the composite prompt-version string persisted on every analysis row.
  - Implement the synchronous orchestrator: load the extraction by id, reject when the extraction is missing or not in done status, run the three LLM completions sequentially, parse and validate the thesis envelope, derive the persisted model from the thesis call's response, and upsert.
  - Implement the JSON-envelope parser that decodes the thesis response into a typed shape and validates that flag is a boolean and rationale is a non-empty string; tolerate extra fields.
  - Add unit tests against an in-memory analyzer repository and the test double from 1.3 covering: happy path returns the persisted analysis, transport error from each of the three calls returns the upstream sentinel without writing any row, malformed envelope (parse failure, missing field, wrong type) returns the malformed sentinel without writing any row, missing extraction returns extraction-not-found and never invokes the LLM, extraction-not-done returns extraction-not-ready and never invokes the LLM.
  - Done when all listed unit-test cases pass and no test path performs a retry.
  - _Requirements: 1.1, 1.4, 1.5, 4.1, 4.2, 4.4, 5.1, 5.2, 5.3, 6.1, 6.2, 6.3, 6.4_
  - _Boundary: application/analyzer_
  - _Depends: 2.1, 1.3_

- [x] 2.3 (P) Implement the analyzer persistence repository with race-safe upsert
  - Define the GORM model with a TableName pin, the columns from the design's physical data model, and From/ToDomain conversions that keep GORM types out of the domain package.
  - Implement the repository's Upsert as a transaction that inserts and, on duplicated-key error, performs an explicit UPDATE via map[string]any so zero-values land; preserve created_at and advance updated_at; return the row as persisted. Mirror the precedent established by the extraction repository — do not introduce clause.OnConflict.
  - Implement FindByID that returns the analysis-not-found sentinel for misses and wraps any other error as catalogue-unavailable.
  - Add integration tests using the project's test-DB helper covering: insert path returns the row, second upsert overwrites content and updated_at while preserving created_at, two concurrent goroutines upserting the same extraction id leave exactly one row whose contents match one of the two writes, FindByID returns analysis-not-found for an unknown id, errors from a closed DB surface as catalogue-unavailable.
  - Done when the listed integration tests pass against a real SQLite database created via the project's test helper.
  - _Requirements: 1.2, 2.1, 2.2, 3.1, 3.2, 3.3, 3.4, 3.5, 5.5_
  - _Boundary: infrastructure/persistence/analyzer_
  - _Depends: 2.1_

- [x] 2.4 (P) Implement the fake LLM provider keyed by prompt version
  - Implement the shared LLMClient port at infrastructure/llm/fake; switch on the request's prompt version to return canned text for short.v1 and long.v1 and a valid thesis envelope for thesis.v1; default to a documented fixed string for unknown versions; always return a stable model identifier such as "fake".
  - Add a unit test that runs each prompt-version branch through the application's envelope parser to confirm the thesis canned output parses cleanly, the canned model identifier is stable, and the same prompt version always returns the same output.
  - Done when the unit test passes and no branch performs network I/O.
  - _Requirements: 7.1, 7.2, 7.3_
  - _Boundary: infrastructure/llm/fake_

- [ ] 3. Integration: HTTP surface and bootstrap wiring

- [x] 3.1 Build the analyzer HTTP controller with Swagger annotations
  - Implement the POST /analyses handler: bind the request body, return 400 on missing or empty extraction_id without invoking the use case, otherwise call the use case and render the persisted analysis under common.Data on success.
  - Implement the GET /analyses/:extraction_id handler: forward to the use case's read path and render the persisted analysis under common.Data, never invoking the LLM.
  - Forward every non-400 failure via c.Error so the existing error-envelope middleware translates the wrapped HTTPError; rely on the new reason field to disambiguate the two 502 modes.
  - Add Swagger annotations for both endpoints following the project's existing controller convention; declare success and every documented failure code (400, 401, 404, 409, 500, 502).
  - Add controller tests using gin's test mode and httptest with a stubbed use case asserting: 200 with the documented response shape on POST and on GET, 400 on empty body, 404 from each not-found sentinel, 409 from extraction-not-ready, 500 with no details.reason from catalogue-unavailable, 502 with details.reason equal to llm_upstream from the upstream sentinel, 502 with details.reason equal to llm_malformed_response from the malformed sentinel.
  - Done when every listed status-code case is asserted by a passing test.
  - _Requirements: 1.3, 2.1, 2.2, 2.3, 4.3, 5.4, 5.5, 8.3_
  - _Boundary: internal/http/controller/analyzer_
  - _Depends: 1.1, 2.2_

- [x] 3.2 Register the analyzer routes on the authenticated /api group
  - Add the analyzer route file alongside existing per-feature route files; register POST /analyses and GET /analyses/:extraction_id on the /api group so they inherit the existing X-API-Token middleware without introducing any new auth surface.
  - Extend the existing route Deps struct with the analyzer use case so the router can construct the controller.
  - Add a route-level test that asserts a request with a missing or wrong token receives the same status the rest of /api uses for unauthenticated requests, and that the analyzer endpoints are reachable when authenticated.
  - Done when the integration-style route test confirms unauthenticated requests are rejected and authenticated requests reach the controller.
  - _Requirements: 8.1, 8.2_
  - _Boundary: internal/http/route_
  - _Depends: 3.1_

- [ ] 3.3 Bootstrap wiring: AutoMigrate, provider selection, dependency injection
  - Append the analyzer persistence model to the project's AutoMigrate list so the analyses table is created at startup.
  - Construct the production fake LLM client when the configured provider equals fake; refuse to start with a clear error when the provider equals the reserved anthropic value until that adapter ships.
  - Build the analyzer repository, build the analyzer use case (passing the extraction repository, the LLM client, the logger, and the clock), wire the analyzer use case onto the route Deps struct, and call route setup so the HTTP surface is mounted.
  - Regenerate Swagger docs by running the project's documented swag task after annotations land.
  - Done when starting the application against a fresh database creates the analyses table and the two endpoints are served under /api.
  - _Requirements: 7.4, 8.1_
  - _Boundary: internal/bootstrap_
  - _Depends: 1.2, 2.3, 2.4, 3.2_

- [ ] 4. Validation: end-to-end integration test

- [ ] 4.1 End-to-end integration test through the full HTTP, use-case, and persistence path
  - Add a new integration test under tests/integration that boots the real bootstrap wiring with the production fake LLM client and a SQLite database created via the project's test-DB helper.
  - Cover the success path: seed an extraction with status done, POST /analyses with its id, assert 200 and the documented response shape, assert exactly one row in the analyses table whose contents match the response.
  - Cover the overwrite path: POST /analyses again with the same id, assert 200, assert the row count is still one, assert created_at is unchanged from the first POST, assert updated_at advanced.
  - Cover the read-back: GET /analyses/:extraction_id returns the same shape and values; an unknown id returns 404.
  - Cover the not-ready path: seed an extraction with a non-done status, POST /analyses, assert 409 and assert no row is written.
  - Done when the integration test passes against the real bootstrap wiring with no flakes across at least two consecutive runs.
  - _Requirements: 1.1, 1.2, 1.3, 1.4, 2.1, 2.2, 3.1, 3.2, 3.3, 3.4, 4.1, 4.2, 4.4, 5.5, 8.1_
  - _Boundary: tests/integration_
  - _Depends: 3.3_
