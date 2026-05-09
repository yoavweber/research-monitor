# Requirements Document

## Introduction

Users of the Research Monitor backend trigger arXiv ingestion through
`POST /api/arxiv/fetch`. Today the endpoint persists paper metadata but
does not download the PDFs whose URLs it captured, leaving downstream
PDF-dependent work (reading, extraction, analysis) blocked.

This feature schedules PDF downloads automatically after each fetch,
returns a job handle to the client without blocking on the downloads,
and exposes per-entry download outcomes through a Server-Sent Events
stream and a REST status endpoint. Failures are isolated per entry so a
single bad URL never aborts the batch. A future flow where the user
reviews the list and explicitly confirms the download is out of scope
for this spec but must remain visible as a deferred decision in the
trigger surface.

## Boundary Context

- **In scope**:
  - Scheduling PDF downloads for newly persisted (`IsNew == true`)
    arXiv entries returned by `/api/arxiv/fetch`.
  - Returning a job identifier and the list of `source_id`s scheduled
    for download as part of the fetch response.
  - Background execution of per-entry downloads with isolated failure
    handling.
  - A per-job Server-Sent Events stream of progress and summary events,
    with replay of events that fired before the subscriber attached.
  - A REST endpoint returning the per-job aggregate status and per-entry
    results.
  - In-memory job state with bounded retention after completion.

- **Out of scope**:
  - Re-downloading entries where `IsNew == false`.
  - A user-confirmation step ("show list, click OK, then download");
    must be marked as a deferred decision but not implemented.
  - Persistence of job state or download outcomes across process
    restarts.
  - Retry, backoff, or rate-limiting policies beyond what the existing. although short comment suggesting to add it is accepted
    `pdf.Store` already provides.

  - WebSocket transport, alternative push channels, or push to anything
    other than the requesting client.
  - Authentication or authorization specific to the new endpoints
    beyond whatever middleware already applies to the arxiv routes.
  - Frontend changes.

- **Adjacent expectations**:
  - The arxiv fetch flow continues to be the single source of trigger
    for this feature; the feature does not poll or subscribe to other
    sources.
  - The existing PDF artifact store provides idempotent fetch +
    publish semantics; this feature relies on those guarantees and
    does not duplicate them.
  - Downstream consumers (e.g. extraction) are not modified by this
    spec; they will continue to access PDFs through the existing
    artifact store.

## Requirements

### Requirement 1: Automatic Download Trigger After Fetch

**Objective:** As a Research Monitor API client, I want PDF downloads to
start automatically after a successful arXiv fetch, so that I do not
need a second manual step to materialize PDF bytes.

#### Acceptance Criteria

1. When `POST /api/arxiv/fetch` completes persistence with at least one
   entry whose `IsNew` is true, the Research Monitor backend shall
   schedule a download job covering exactly those `IsNew` entries.
2. When `POST /api/arxiv/fetch` completes persistence with zero entries
   whose `IsNew` is true, the Research Monitor backend shall not
   schedule a download job and shall return a fetch response that
   indicates no entries were scheduled.
3. If `POST /api/arxiv/fetch` returns a non-success status because the
   fetch or persistence step failed, the Research Monitor backend shall
   not schedule any download job for that request.
4. The Research Monitor backend shall expose, at the trigger site, a
   visible deferred decision noting that a future flow may move the
   download trigger behind an explicit user confirmation step.

### Requirement 2: Non-Blocking Fetch Response With Job Handle

**Objective:** As a Research Monitor API client, I want the fetch
endpoint to return immediately with a handle that lets me observe
download progress, so that long-running downloads never delay my
fetch response.

#### Acceptance Criteria

1. When a download job is scheduled for a fetch request, the Research
   Monitor backend shall return the fetch response without waiting for
   any PDF download to complete.
2. When a download job is scheduled, the Research Monitor backend shall
   include in the fetch response a unique job identifier and the list
   of `source_id`s that were scheduled for download.
3. The Research Monitor backend shall ensure that a job identifier
   returned to one fetch caller cannot collide with a job identifier
   returned to any other caller within the configured retention window.
4. While a download job is in progress, the Research Monitor backend
   shall continue to accept and process additional `POST
   /api/arxiv/fetch` requests independently of that job.

### Requirement 3: Per-Entry Download Execution and Failure Isolation

**Objective:** As a Research Monitor API client, I want every scheduled
entry to be attempted independently, so that a single failing URL does
not prevent other PDFs from being downloaded.

#### Acceptance Criteria

1. While a download job is in progress, the Research Monitor backend
   shall attempt to fetch the PDF bytes for each scheduled entry in the
   job.
2. When a per-entry download succeeds, the Research Monitor backend
   shall record the entry's outcome as `success` together with the
   number of bytes fetched.
3. If a per-entry download fails, the Research Monitor backend shall
   record the entry's outcome as `failed` together with a stable
   failure category and a human-readable error description, and shall
   continue processing the remaining entries in the job.
4. The Research Monitor backend shall ensure that scheduling the same
   entry more than once across overlapping fetch calls does not produce
   duplicate PDF artifacts.
5. The Research Monitor backend shall ensure that a client cancelling
   or disconnecting from the fetch request does not cancel an
   already-scheduled download job.

### Requirement 4: Per-Job Server-Sent Events Stream With Replay

**Objective:** As a Research Monitor API client, I want a per-job event
stream that I can subscribe to without missing events, so that I can
display progress and failures to the end user in near real time.

#### Acceptance Criteria

1. When a client opens `GET /api/arxiv/downloads/{job_id}/stream` for a
   known job, the Research Monitor backend shall stream all events
   recorded for that job so far before forwarding new events.
2. When a per-entry download completes (success or failure), the
   Research Monitor backend shall publish a `download.progress` event
   on the job's stream containing the entry's `source_id`, status, and
   either fetched-byte count or failure category and description.
3. When all scheduled entries in a job have completed, the Research
   Monitor backend shall publish a single terminal `download.summary`
   event containing total, succeeded, and failed counts, and shall
   close the stream after sending it.
4. If a client disconnects from a job stream, the Research Monitor
   backend shall stop writing to that client without affecting the
   underlying job or other subscribers of the same job.
5. If a client requests a stream for an unknown or expired job
   identifier, the Research Monitor backend shall respond with a
   client-error status without opening a stream.
6. While a job stream is open, the Research Monitor backend shall not
   require the client to send any data to keep receiving events.

### Requirement 5: Per-Job REST Status Endpoint

**Objective:** As a Research Monitor API client that missed or cannot
use the event stream, I want a REST endpoint that returns the same
per-job results, so that I can poll or recover state after a
disconnect.

#### Acceptance Criteria

1. When a client requests `GET /api/arxiv/downloads/{job_id}` for a
   known job, the Research Monitor backend shall return the job
   identifier, the totals (total, succeeded, failed), a completion
   indicator, and a per-entry result list with `source_id`, status, and
   either bytes fetched or failure category and description.
2. While a download job is still in progress, the Research Monitor
   backend shall return per-entry results for entries already finished
   and shall mark the job as not yet complete.
3. If a client requests status for an unknown or expired job
   identifier, the Research Monitor backend shall respond with a
   client-error status indicating the job is not available.
4. The Research Monitor backend shall ensure that the per-entry
   results returned by the status endpoint are consistent with the
   events published on the corresponding job stream.

### Requirement 6: Job Retention and Bounded Memory

**Objective:** As a Research Monitor operator, I want job state to be
retained long enough for clients to observe results but bounded in
memory, so that long-running processes do not accumulate stale state.

#### Acceptance Criteria

1. While a download job is in progress, the Research Monitor backend
   shall retain its full state and event history in memory.
2. When a download job completes, the Research Monitor backend shall
   retain its state and event history for at least the configured
   retention window before discarding it.
3. After the retention window has elapsed for a completed job, the
   Research Monitor backend shall discard the job's state and shall
   treat subsequent stream or status requests for that job identifier
   as unknown.
4. When the backend process restarts, the Research Monitor backend
   shall treat all previously-issued job identifiers as unknown.
5. The Research Monitor backend shall log job lifecycle transitions
   (scheduled, completed, evicted) in a way that an operator can
   correlate with the originating fetch request.
