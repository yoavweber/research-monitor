# Requirements Document

## Project Description (Input)

**Who has the problem**: The researcher running this personal DeFi research monitor — the sole user of the backend. They need the papers returned by `/api/arxiv/fetch` to be retained across sessions, with enough identity information that a second paper source added later cannot collide on identifier, so later pipeline stages (cross-fetch dedupe, triage, LLM summarisation, frontend feed) have a persistent, source-aware catalogue to read from.

**Current situation**: `paper.Entry` exists as an immutable domain value object delivered by the `arxiv-fetcher` spec, but nothing persists it and nothing carries its provenance. `/api/arxiv/fetch` returns entries in the HTTP response only, and that spec's requirement 1.4 explicitly forbids the endpoint from writing to any datastore. The precedent for a persistence adapter lives at `internal/infrastructure/persistence/source/` — separate persistence model, `ToDomain`/`FromDomain` conversion, unexported struct, interface-returning constructor, auto-migration wired in bootstrap. No equivalent exists for `paper.Entry`. `arxiv-fetcher`'s design lists persistence as out-of-boundary and as an explicit revalidation trigger; this spec is that revalidation.

**What should change**: Extend `paper.Entry` with a `Source` field (the arxiv ingestion path sets it to `"arxiv"`), and add an idempotent save capability keyed on the composite `(Source, SourceID)` with first-seen-wins semantics. The uniqueness invariant is enforced at the storage layer itself so concurrent save attempts cannot produce duplicates. Two authenticated read-only HTTP endpoints expose the persisted catalogue: `GET /api/papers` (list, newest-first) and `GET /api/papers/:source/:source_id` (single-paper retrieval). `GET /api/arxiv/fetch` is modified to auto-persist every successfully fetched entry before returning its response; each returned entry additionally carries a boolean `is_new` flag distinguishing newly persisted entries from duplicates that were skipped. `arxiv-fetcher`'s requirement 1.4 is superseded by this spec. Out of scope: actually introducing a second paper source in v1 (the schema and API are source-aware, but only the arxiv source is wired today), version-upgrade semantics beyond first-seen-wins, delete/update endpoints, pagination, per-source category or config routing, PDF extraction, LLM summarisation, triage, frontend, scheduling.

## Introduction

The paper-persistence feature gives the research monitor a durable, source-aware catalogue of every `paper.Entry` that the system ingests. It owns three things: the storage contract (`Save`, `Find`, `List` keyed on `(Source, SourceID)`), the storage invariant that prevents duplicate composite keys even under concurrent saves, and the read-only HTTP surface needed to inspect the catalogue. It also takes over the ingestion seam that `arxiv-fetcher` deliberately left open: the arxiv fetch endpoint now persists on the way out and annotates each returned entry with an `is_new` flag. The spec repeals `arxiv-fetcher` requirement 1.4 as part of this change.

## Boundary Context

- **In scope**:
  - Extending `paper.Entry` with a `Source` field; the arxiv ingestion path sets `Source = "arxiv"` on every entry.
  - Idempotent save of a `paper.Entry` into the catalogue, keyed on the composite `(Source, SourceID)` with first-seen-wins semantics.
  - Retrieval of a single persisted paper by `(Source, SourceID)` through a path-nested endpoint.
  - Retrieval of the full catalogue, newest-first by submission date.
  - A storage invariant that makes duplicate `(Source, SourceID)` pairs impossible even under concurrent save attempts.
  - Two authenticated read-only HTTP endpoints that expose the single-paper and full-list views.
  - Modifying `GET /api/arxiv/fetch` to persist every successfully fetched entry synchronously, before its response is written, and to include a per-entry `is_new` boolean in the response body.
  - Repealing `arxiv-fetcher` requirement 1.4 and updating that spec's integration tests to reflect the new auto-persist behaviour.
  - A fail-fast startup contract: the service refuses to accept requests if the catalogue's storage surface is not ready.
- **Out of scope**:
  - Any HTTP endpoint that accepts a direct paper-save request (no `POST`, `PUT`, `PATCH`, or `DELETE` under `/api/papers`). The only save path remains `/api/arxiv/fetch`.
  - Actually introducing a second concrete paper source in v1 (the schema and API shape are source-aware from day one, but only the arxiv source is wired).
  - Per-source configuration or routing (no "enable/disable biorxiv" setting, no per-source rate limits, no per-source category translation).
  - Version-upgrade semantics: a later revision of a paper already in the catalogue is skipped (first-seen wins); it is never merged, upserted, or appended as a second entry.
  - Deletion, pruning, housekeeping, or in-place edits of persisted papers.
  - Pagination, filtering, or sort controls beyond the default newest-first order.
  - PDF fetching, HTML extraction, LLM summarisation, triage, frontend UI, scheduling, cron, or background loops.
- **Adjacent expectations**:
  - The authenticated `/api` group mounts the existing static-token middleware (`X-API-Token`); this spec inherits that protection rather than reauthoring it.
  - `paper.Entry` is defined upstream by the `arxiv-fetcher` spec; this spec extends that type by adding a single `Source` field without changing any other field's semantics.
  - `arxiv-fetcher` requirement 1.4 ("shall not write fetched entries to any datastore") is superseded by this spec. The arxiv-fetcher integration tests that asserted zero datastore writes are updated accordingly as part of this spec's work.

## Requirements

### Requirement 1: Idempotent source-aware save

**Objective:** As the researcher ingesting papers across multiple sessions, I want calling save with an already-persisted paper to be a no-op, with identity keyed on the paper's source and source-side identifier together, so that the catalogue accumulates monotonically and two different sources cannot collide on the same local identifier.

#### Acceptance Criteria

1. When save is called with a paper whose `(Source, SourceID)` pair is not yet present in the catalogue, the paper-persistence service shall record the paper and report that a new entry was created.
2. When save is called with a paper whose `(Source, SourceID)` pair is already present in the catalogue, the paper-persistence service shall leave the existing entry unchanged and report that the paper was skipped.
3. When save is called with a paper that shares a `SourceID` with an existing entry but differs in `Source`, the paper-persistence service shall treat the two as distinct entries and persist both.
4. The paper-persistence service shall preserve all fields of the provided `paper.Entry` exactly as received — `Source`, `SourceID`, `Version`, `Title`, `Authors`, `Abstract`, `PrimaryCategory`, `Categories`, `SubmittedAt`, `UpdatedAt`, `PDFURL`, `AbsURL` — with no field normalised, trimmed, or derived during save.
5. The paper-persistence service shall distinguish, to the caller, between the "newly created" and "skipped" outcomes for every successful save, so stored-versus-skipped counts are observable at every call site.
6. When save is called multiple times with the same paper, the externally observable state of the catalogue shall be identical to the state after a single call.

### Requirement 2: Retrieve a single persisted paper

**Objective:** As the researcher, I want to check whether a specific paper from a specific source has been persisted and read its stored fields on demand, so that I can confirm ingest status and inspect individual entries without scanning the full catalogue.

#### Acceptance Criteria

1. When an authenticated client issues `GET /api/papers/:source/:source_id` with a pair that is present in the catalogue, the paper-persistence service shall return an HTTP 200 response carrying the stored paper's full field set (including its `Source`).
2. If an authenticated client issues `GET /api/papers/:source/:source_id` with a pair that is not present in the catalogue, the paper-persistence service shall return an HTTP 404 response. This behaviour shall not depend on whether the `:source` value names a known ingestion source — an unknown source yields 404 rather than a validation error, because the catalogue is the authority on what exists.
3. If the request omits or presents an invalid `X-API-Token`, the paper-persistence service shall return an HTTP 401 response and shall not access the catalogue.
4. The returned paper's field values shall be identical to the values recorded at save time; no field is re-derived or mutated on read.

### Requirement 3: List the catalogue

**Objective:** As the researcher, I want to see every paper persisted so far, ordered newest-first by submission date, so that I can scan the catalogue in the order that matches my research workflow.

#### Acceptance Criteria

1. When an authenticated client issues `GET /api/papers`, the paper-persistence service shall return an HTTP 200 response containing every paper currently in the catalogue, regardless of source.
2. When papers are returned, they shall be ordered by their submission date (`SubmittedAt`) in descending order (newest first).
3. When the catalogue is empty, the paper-persistence service shall return an HTTP 200 response with an empty list rather than an error.
4. If the request omits or presents an invalid `X-API-Token`, the paper-persistence service shall return an HTTP 401 response and shall not access the catalogue.
5. The fields exposed for each paper in the list shall be identical to the fields exposed by the single-paper retrieval endpoint (the list is a collection of the same item shape, `Source` included).

### Requirement 4: Storage invariant and startup readiness

**Objective:** As the operator, I want the composite-key uniqueness rule to be guaranteed across all code paths and the service to refuse to start when its persistence surface is not ready, so that the catalogue cannot be corrupted by a missed application-layer check and so a broken startup state is loud rather than silent.

#### Acceptance Criteria

1. The paper-persistence service shall guarantee that at most one entry exists for any given `(Source, SourceID)` pair in the catalogue, regardless of how save attempts arrive (serialised, concurrent, or through any internal path that reaches the storage).
2. If two distinct save attempts with the same `(Source, SourceID)` pair are received concurrently, the paper-persistence service shall accept exactly one and signal the other as skipped; at no intermediate point shall two entries with the same pair be visible to a retrieval or list request.
3. The paper-persistence service shall be ready to serve save, retrieve, and list requests as soon as it begins accepting requests; no request shall be rejected because the catalogue's storage was not yet initialised.
4. If the service cannot initialise the catalogue's storage during startup, it shall fail to start and report the failure rather than accepting any request.

### Requirement 5: Auto-persistence on arxiv fetch

**Objective:** As the researcher, I want every successful arxiv fetch to also persist its entries into the catalogue as a single atomic action, with the response telling me which entries are newly persisted and which were already there, so that I can trigger a fetch once and read an annotated result without a second roundtrip.

#### Acceptance Criteria

1. When an authenticated client successfully calls `GET /api/arxiv/fetch`, the paper-persistence service shall persist every returned entry into the catalogue before the response is written.
2. When an entry returned by a fetch is persisted as a new row, the response representation of that entry shall include the field `is_new` set to `true`.
3. When an entry returned by a fetch is skipped because `(Source, SourceID)` already exists in the catalogue, the response representation of that entry shall include the field `is_new` set to `false`; the entry is still returned in the response body.
4. When the arxiv fetch is triggered, every returned entry's `Source` field shall be set to `"arxiv"`.
5. If persistence fails for any entry returned by the fetch (for reasons other than the dedupe skip defined in acceptance criterion 3), the `/api/arxiv/fetch` request shall return an HTTP 5xx error response and shall not write a partial or fabricated entry list in the response body.
6. The auto-persistence behaviour defined by this requirement supersedes `arxiv-fetcher` requirement 1.4; that requirement is repealed as part of this spec and the corresponding "no datastore writes" integration test is removed or inverted in the same change.
7. The ordering and count of entries in the `/api/arxiv/fetch` response body shall be unchanged by auto-persistence — the response contains exactly the entries returned upstream, in their upstream order, each annotated with `Source` and `is_new`.
