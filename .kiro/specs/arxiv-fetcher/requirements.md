# Requirements Document

## Project Description (Input)

**Who has the problem**: The researcher running this personal DeFi research monitor — the sole user of the backend. They need arXiv papers pulled into their pipeline so they can later summarise, triage, and surface thesis-angle candidates.

**Current situation**: The product defines a generic `APIFetcher` port in `internal/domain/shared/ports.go` for JSON API ingestion sources (arXiv, governance forums), but no concrete implementation exists. Nothing in the codebase talks to arXiv.

**What should change**: Add a concrete arXiv implementation of the `APIFetcher` port plus a manually triggered HTTP endpoint that runs a fetch and returns the raw arXiv entries in the response. The set of arXiv categories to query is fixed via environment configuration. This spec does **not** cover article persistence, dedupe, PDF extraction, LLM summarisation, or scheduling.

## Introduction

The arxiv-fetcher feature gives the researcher a manual, on-demand way to pull the most recently submitted arXiv papers for a fixed list of categories and receive them in the HTTP response. No state is written; each trigger returns a fresh, chronologically ordered snapshot of the top-of-feed for the configured categories. This is the first concrete ingestion adapter in the monitor and establishes the contract that later pipeline stages (dedupe, extraction, summarisation, persistence) will consume, but those stages are not in scope here.

## Boundary Context

- **In scope**:
  - A manual HTTP endpoint that triggers a fetch and returns fetched arXiv entries.
  - Querying arXiv for a fixed, configured set of categories.
  - Sorting entries by submission date, newest first.
  - Returning at most a configured number of entries per call (single page).
  - Distinguishing upstream failures (arXiv unavailable, malformed, timeout) from internal errors in the response.
- **Out of scope**:
  - Persisting fetched entries to any datastore.
  - Deduplication against prior fetches.
  - Article aggregate, PDF extraction, HTML extraction, LLM summarisation.
  - Scheduling, cron, or background periodic fetches.
  - Per-user or per-`Source`-row category configuration; this feature does **not** integrate with the existing `Source` aggregate.
  - Rate-limit enforcement beyond arXiv's own responses (manual trigger only).
- **Adjacent expectations**:
  - The endpoint is protected by the monitor's existing static-token authentication (`X-API-Token`). The fetcher does not own authentication; it expects the authenticated-request contract already established for the `/api/*` surface.
  - A future spec owns article persistence and dedupe; this feature's response shape should be rich enough that a later pipeline stage can identify and dedupe papers, but it must not assume anything about how that stage stores data.

## Requirements

### Requirement 1: Manual arXiv fetch trigger

**Objective:** As the researcher, I want to trigger an arXiv fetch on demand via an HTTP request, so that I can pull the latest papers for my configured categories whenever I want without relying on a scheduler.

#### Acceptance Criteria

1. When an authenticated client issues the arXiv fetch request, the arxiv-fetcher shall return an HTTP 200 response with a JSON body containing the fetched arXiv entries.
2. If the request omits or presents an invalid `X-API-Token`, the arxiv-fetcher shall return an HTTP 401 response and shall not contact arXiv. the solution for this should be generic for any future protected endpoint
3. When the response is returned, the arxiv-fetcher shall include, for each entry, enough information to identify the paper: at minimum the arXiv identifier, title, authors, abstract, primary category, submission date, and a link to the PDF.
4. ~~The arxiv-fetcher shall not write fetched entries to any datastore as part of handling the request.~~ **Superseded by `paper-persistence` spec (Requirement 5: Auto-persistence on arxiv fetch).** The fetch endpoint now persists every returned entry into the catalogue before responding and annotates each entry with `is_new`.
5. When no entries match the configured categories, the arxiv-fetcher shall return an HTTP 200 response with an empty entry list rather than an error.

### Requirement 2: Category configuration

**Objective:** As the operator, I want the arXiv category list to be fixed through environment configuration, so that the fetch scope is deterministic, reproducible across restarts, and not accidentally mutable at runtime.

#### Acceptance Criteria

1. The arxiv-fetcher shall read the configured arXiv category list from environment configuration at startup.
2. If the configured category list is missing or empty at startup, the arxiv-fetcher shall fail to start and shall report the missing configuration.
3. When a fetch is triggered, the arxiv-fetcher shall query arXiv using only the categories present in the configured list.
4. When the configured list contains multiple categories, the arxiv-fetcher shall return entries matching any one of those categories.
5. The arxiv-fetcher shall not expose any interface for adding, removing, or mutating the configured category list at runtime.

### Requirement 3: Query behavior

**Objective:** As the researcher, I want each fetch to return the most recently submitted papers in a bounded, predictable window, so that I can review newest work first and the response size stays reasonable.

#### Acceptance Criteria

1. When a fetch is triggered, the arxiv-fetcher shall request entries sorted by submission date in descending order.
2. When a fetch is triggered, the arxiv-fetcher shall request at most the configured `max_results` entries in a single call.
3. If the configured `max_results` is missing, zero, negative, or non-numeric at startup, the arxiv-fetcher shall fail to start and shall report the misconfiguration.
4. The arxiv-fetcher shall not perform additional paginated requests beyond the single configured page in a single trigger.
5. When two fetches are triggered in succession without new submissions to arXiv in between, the arxiv-fetcher shall return the same set of entries (same order, same identifiers).

### Requirement 4: Upstream failure handling

**Objective:** As the operator, I want clear and distinguishable failure signals when arXiv is unavailable or returns unexpected data, so that I can tell transient upstream issues apart from internal bugs without reading logs.

#### Acceptance Criteria

1. If arXiv returns a non-success HTTP status, the arxiv-fetcher shall return an HTTP 502 response with a message indicating an upstream arXiv failure.
2. If the arXiv response body cannot be parsed as the expected arXiv format, the arxiv-fetcher shall return an HTTP 502 response with a message indicating a malformed upstream response.
3. If the request to arXiv times out or the network call fails before a response is received, the arxiv-fetcher shall return an HTTP 504 response.
4. When any upstream failure occurs, the arxiv-fetcher shall not return a partial or fabricated entry list in the response body.
5. When a fetch completes (successfully or with an upstream failure), the arxiv-fetcher shall log the outcome, including the failure category when applicable.
