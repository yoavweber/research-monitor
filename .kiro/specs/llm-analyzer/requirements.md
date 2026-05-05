# Requirements Document

## Introduction

The `llm-analyzer` feature turns an already-extracted paper's body markdown into three LLM-derived artifacts — a short summary, a long summary, and a thesis-angle classification — and persists them for later retrieval. The sole-user researcher uses these artifacts to triage papers without reading every body in full and to surface candidates worth deeper thesis exploration.

Analysis is synchronous, keyed by `extraction_id`, and overwrite-on-rerun. The first slice ships with a fake LLM provider so the path is exercisable end-to-end without an API key; the real provider implementation is a follow-up spec. All failure modes (missing extraction, unfinished extraction, LLM transport error, malformed thesis-angle response) fail fast with explicit HTTP status codes and no retries.

Full scope, boundaries, response contract, upsert semantics, and out-of-scope items are documented in [brief.md](./brief.md).

## Boundary Context

- **In scope**:
  - Synchronous `POST /analyses {extraction_id}` that runs three LLM calls and persists the result.
  - `GET /analyses/:extraction_id` for read-back.
  - Re-running for the same `extraction_id` overwrites the stored row while preserving `created_at`.
  - A default fake LLM provider whose output is deterministic per prompt type, including a valid thesis-angle JSON envelope.
  - Typed failure handling for missing extraction, extraction not in `done` status, LLM transport error, and malformed thesis-angle response.

- **Out of scope**:
  - Real LLM provider integration (Anthropic or other). Deferred to a follow-up.
  - Asynchronous, queued, scheduled, or background-worker analysis.
  - Analysis history, versioning, or multi-revision storage.
  - Automatic re-analysis triggered by prompt-version changes.
  - Listing or filtering endpoints (e.g., index, search, by-source). Only by-id retrieval is in scope.
  - Triage / news-vs-paper classification. Thesis-angle is the only classification.
  - Cost, token, or rate-limit accounting.
  - Reads from the papers store; analyzer never depends on `paper-persistence`.
  - Retry / backoff of any kind for LLM calls.

- **Adjacent expectations**:
  - `document-extraction` is expected to publish the body markdown for an extraction once its status reaches `done`. The analyzer treats any other status as not-ready and any unknown id as not-found.
  - The shared LLM port already exists at the project level. The analyzer is its first consumer; the analyzer's prompt strings and response-parsing rules are owned inside this feature, not by the port.
  - Authentication uses the existing project-wide single-token scheme; this feature introduces no new auth mechanism.

## Requirements

### Requirement 1: Submit analysis for an extraction

**Objective:** As the researcher, I want to request an LLM analysis for a specific extracted paper, so that I receive its short summary, long summary, and thesis-angle classification in a single synchronous call.

#### Acceptance Criteria

1. When the researcher submits an authenticated `POST /analyses` request whose body contains a valid `extraction_id` referring to an extraction in `done` status, the Analyzer Service shall produce a short summary, a long summary, a thesis-angle boolean flag, and a thesis-angle rationale derived from that extraction's body markdown.
2. When analysis production succeeds, the Analyzer Service shall persist the resulting analysis keyed by `extraction_id` before returning the response.
3. When analysis production succeeds, the Analyzer Service shall respond with HTTP `200 OK` and a JSON body containing `extraction_id`, `short_summary`, `long_summary`, `thesis_angle_flag`, `thesis_angle_rationale`, `model`, `prompt_version`, `created_at`, and `updated_at`.
4. While a `POST /analyses` request is in flight, the Analyzer Service shall hold the HTTP response open until the analysis is fully produced and persisted, with no asynchronous or queued completion.
5. The Analyzer Service shall produce the short summary, long summary, and thesis-angle classification through three independent LLM invocations rather than a single combined call.

### Requirement 2: Retrieve a stored analysis

**Objective:** As the researcher, I want to fetch a previously produced analysis by extraction id, so that I can re-display summaries without paying for another LLM run.

#### Acceptance Criteria

1. When the researcher submits an authenticated `GET /analyses/:extraction_id` request and an analysis exists for that `extraction_id`, the Analyzer Service shall respond with HTTP `200 OK` and the same JSON shape returned by `POST /analyses`.
2. If the researcher submits an authenticated `GET /analyses/:extraction_id` request and no analysis exists for that `extraction_id`, then the Analyzer Service shall respond with HTTP `404 Not Found`.
3. The Analyzer Service shall not invoke any LLM call in response to a `GET /analyses/:extraction_id` request.

### Requirement 3: Re-run overwrites prior analysis

**Objective:** As the researcher, I want to re-submit an analysis request for the same extraction and have the latest result replace the previous one, so that I can refresh stale summaries without managing history myself.

#### Acceptance Criteria

1. When the researcher submits a `POST /analyses` request for an `extraction_id` that already has a stored analysis, the Analyzer Service shall replace the stored `short_summary`, `long_summary`, `thesis_angle_flag`, `thesis_angle_rationale`, `model`, and `prompt_version` with values from the new run.
2. When the Analyzer Service replaces an existing analysis, it shall preserve the original `created_at` value of the prior row.
3. When the Analyzer Service replaces an existing analysis, it shall advance `updated_at` to the time of the new run.
4. The Analyzer Service shall keep at most one stored analysis per `extraction_id` at any time.
5. If two `POST /analyses` requests for the same `extraction_id` are processed concurrently, then the Analyzer Service shall leave exactly one stored analysis for that `extraction_id` whose contents correspond to one of the two runs, without producing duplicate stored analyses and without surfacing a storage conflict to the researcher.

### Requirement 4: Extraction precondition validation

**Objective:** As the researcher, I want clear failures when I request analysis for an extraction that does not exist or is not yet ready, so that I can distinguish "wrong id" from "not finished" without reading server logs.

#### Acceptance Criteria

1. If the researcher submits a `POST /analyses` request for an `extraction_id` that does not correspond to any known extraction, then the Analyzer Service shall respond with HTTP `404 Not Found` and shall not invoke any LLM call.
2. If the researcher submits a `POST /analyses` request for an `extraction_id` whose extraction exists but is not in `done` status, then the Analyzer Service shall respond with HTTP `409 Conflict` and shall not invoke any LLM call.
3. If the `POST /analyses` request body is missing `extraction_id`, contains an empty `extraction_id`, or is not valid JSON, then the Analyzer Service shall respond with HTTP `400 Bad Request` and shall not invoke any LLM call.
4. When the Analyzer Service rejects a `POST /analyses` request under any precondition failure, it shall not write any analysis row.

### Requirement 5: LLM failure handling without retry

**Objective:** As the researcher, I want LLM failures to surface immediately rather than retrying silently, so that I can see real failures in the response and decide whether to retry myself.

#### Acceptance Criteria

1. If any of the three LLM invocations returns a transport-level error, then the Analyzer Service shall respond with HTTP `502 Bad Gateway` and shall not write any analysis row.
2. If the thesis-angle LLM response is not parseable as the required JSON envelope, then the Analyzer Service shall respond with HTTP `502 Bad Gateway` and shall not write any analysis row.
3. The Analyzer Service shall not retry any LLM invocation within a single `POST /analyses` request.
4. When the Analyzer Service produces a `502` response, it shall include a machine-readable indicator distinguishing transport failure from malformed-response failure so that the researcher can tell the two apart from the response alone.
5. If a persistence failure occurs after a successful LLM run, then the Analyzer Service shall respond with HTTP `500 Internal Server Error` and shall not return a partial analysis body.

### Requirement 6: Thesis-angle response contract

**Objective:** As the researcher, I want the thesis-angle classification to be a structured boolean plus rationale, so that downstream consumers can filter on the flag without parsing prose.

#### Acceptance Criteria

1. The Analyzer Service shall require the thesis-angle LLM response to be a JSON object with a boolean field `flag` and a string field `rationale`, and nothing else.
2. When the thesis-angle LLM response satisfies the required envelope, the Analyzer Service shall map `flag` to the persisted `thesis_angle_flag` and `rationale` to the persisted `thesis_angle_rationale` verbatim.
3. If the thesis-angle LLM response is parseable JSON but is missing `flag`, missing `rationale`, has a non-boolean `flag`, or has a non-string `rationale`, then the Analyzer Service shall treat the response as malformed per Requirement 5.2.
4. The Analyzer Service shall store the short and long summaries as the LLM's textual completions with surrounding whitespace trimmed, with no further structural parsing.

### Requirement 7: Default fake LLM provider

**Objective:** As the operator, I want the analyzer to be exercisable end-to-end without configuring a real LLM provider, so that I can develop and test the full path before any provider integration ships.

#### Acceptance Criteria

1. The Analyzer Service shall ship with a default LLM provider that requires no external network call and no API key to operate.
2. When the default provider is invoked for the short-summary prompt, the long-summary prompt, or the thesis-angle prompt, it shall return canned output that is deterministic for that prompt type.
3. When the default provider is invoked for the thesis-angle prompt, it shall return a response that satisfies the JSON envelope required by Requirement 6.1, so that the success path is exercisable end-to-end.
4. The Analyzer Service shall allow the default provider to be replaced by a different provider implementation in the future without changes to the analyzer's prompt set, response-parsing rules, or persisted analysis shape.

### Requirement 8: Authentication and request hygiene

**Objective:** As the operator, I want the analyzer endpoints to follow the project's existing auth and validation conventions, so that this feature does not introduce a new attack surface or a divergent API style.

#### Acceptance Criteria

1. If a `POST /analyses` or `GET /analyses/:extraction_id` request omits the project-wide authentication token or supplies an incorrect one, then the Analyzer Service shall reject the request with the same HTTP status the rest of the project uses for unauthenticated requests, and shall not invoke any LLM call or write any analysis row.
2. The Analyzer Service shall not introduce additional authentication mechanisms, headers, or token formats beyond those already used by the project.
3. The Analyzer Service shall accept and produce JSON request and response bodies only.
