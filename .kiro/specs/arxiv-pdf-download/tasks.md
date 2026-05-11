# Implementation Plan

## 1. Foundation: domain extensions, configuration, and test doubles

- [x] 1. Domain, configuration, and test infrastructure setup

- [x] 1.1 Extend the paper aggregate with a stable identity value object
  - Add a paper.ID value object that captures (Source, SourceID, Version) with a Validate method that rejects empty Source/SourceID and any path-traversal characters in identity components
  - Add free constructors paper.NewID and paper.IDFromEntry; do not add any method on Entry itself, preserving the existing "Entry carries no behavior" convention
  - Add a (paper.ID).PDFArtifactKey method that returns the source-scoped artifact identifier used as the SourceID of pdf.Key (for arXiv this concatenates SourceID and Version), so the worker and any future callers share one rule
  - Observable completion: package paper compiles, paper.NewID, paper.IDFromEntry, and (paper.ID).PDFArtifactKey are exported, and unit tests cover Validate (empty + path char rejection) and PDFArtifactKey (arXiv versioned + non-versioned cases)
  - _Requirements: 3.4_

- [x] 1.2 Add the PDF-download value objects on the paper aggregate
  - Add paper.PDFDownloadRequest carrying only PaperID and PDFURL with a Validate method that rejects empty PDFURL or invalid PaperID
  - Add paper.NewPDFDownloadRequests that maps a slice of entries (already filtered to IsNew == true by the caller) into the request slice; empty input returns an empty slice
  - Add paper.DownloadJobID, paper.DownloadEntryStatus enum (pending, success, failed), paper.DownloadEntryResult, paper.DownloadJobSnapshot, and the paper.DownloadEvent sum type with exactly one of Progress/Summary populated per emission
  - Observable completion: a unit test in the paper package round-trips a small slice of entries through paper.NewPDFDownloadRequests and asserts the resulting requests carry the expected PaperID and PDFURL, and the value objects are referenceable from outside the package
  - _Requirements: 1.1, 2.2, 3.1, 3.2, 3.3_

- [x] 1.3 Add the PDF-download ports and error sentinel on the paper aggregate
  - Add paper.PDFScheduler with a SchedulePDFDownloads method that documents the non-cancellable contract (state mutation does not consult ctx; empty requests returns a zero snapshot and nil error; non-nil error only on registry shutdown)
  - Add paper.PDFDownloadReader with SnapshotPDFDownloadJob and SubscribePDFDownloadJob, documenting that Subscribe captures backlog and registers the subscriber atomically under the same per-job lock so every event is delivered through exactly one of backlog or live
  - Add paper.ErrDownloadJobUnknown sentinel for both Snapshot and Subscribe
  - Observable completion: package paper compiles, both ports are exported, and a build-time interface compliance check (var _ paper.PDFScheduler = (*tests/mocks.FakePDFScheduler)(nil) once the fake exists) is recorded in the test mocks file added later in 1.6
  - _Requirements: 1.1, 1.2, 2.1, 2.3, 3.5, 4.1, 4.5, 5.1, 5.3, 5.4, 6.3_

- [x] 1.4 Extend bootstrap configuration with retention and subscriber-buffer settings
  - Add PDFDownloadRetention (default 5 minutes) and PDFDownloadSubscriberBuffer (default 32) to the env struct with viper tags
  - Document the defaults in the env struct comments next to the field tags
  - Observable completion: env loads with defaults when the variables are unset and overrides them when they are set; an env unit test asserts both behaviors
  - _Requirements: 6.2_

- [x] 1.5 Add the swag DTO scaffolding for the new download surfaces
  - Define DTOs that mirror paper.DownloadJobSnapshot and paper.DownloadEntryResult for the download controller responses (status envelope, progress event, summary event)
  - Define an extension to the arxiv FetchResponse that carries the initial DownloadJobSnapshot under a job field with omitempty
  - Observable completion: the DTO files compile and are referenced by placeholder swag annotations on the existing arxiv controller (annotations will be filled in when the controller is modified in task 4.1); no behavior change yet
  - _Requirements: 2.2, 5.1_

- [x] 1.6 (P) Add hand-written test doubles for paper.PDFScheduler and pdf.Store
  - Add a fake paper.PDFScheduler under tests/mocks that records calls and returns a configurable snapshot; assert interface compliance at construction time
  - Add a fake pdf.Store under tests/mocks (if not already present) that returns a programmable Locator-or-error per Key, with a hook for blocking sends used by the slow-subscriber test
  - Observable completion: both fakes compile, satisfy their target interfaces (verified via var _ assertions), and have a tiny self-test that demonstrates call recording
  - _Requirements: 1.1, 1.2, 1.3, 3.1, 3.2, 3.3_
  - _Boundary: tests/mocks_

## 2. Core: registry, worker, classifier

- [x] 2. PDF-download orchestration in application/pdfdownload

- [x] 2.1 Build the registry skeleton with Schedule and lifecycle logging
  - Implement an unexported registry struct keyed by paper.DownloadJobID with a single mutex guarding the jobs map, a registry-owned background context with cancel, and a NewRegistry constructor returning the concrete registry plus a ShutdownFunc
  - Implement SchedulePDFDownloads so that registration mutates state without consulting the passed ctx and returns the initial snapshot (Total set, all Entries Pending, Completed=false) atomically; empty requests short-circuits with a zero snapshot
  - Emit pdfdownload.job.scheduled at Info with job_id, total, paper_ids on the shared logger
  - Wire the *Registry construction so the same value satisfies both paper.PDFScheduler and paper.PDFDownloadReader (verified with var _ assertions)
  - Observable completion: a unit test schedules one and many requests, asserts a unique DownloadJobID per call, asserts the returned snapshot shape, and asserts a "scheduled" log line was emitted; an empty-requests test asserts no job is created
  - _Requirements: 1.2, 2.1, 2.2, 2.3, 2.4, 3.5, 6.5_
  - _Boundary: application/pdfdownload_

- [x] 2.2 Implement the per-entry worker with Classifier and pdf.Store
  - Add a classifier function that maps a returned error from pdf.Store.Ensure to (status, category, description) using pdf.ErrInvalidKey/ErrFetch/ErrStore and a default "unknown" bucket; description is sanitized to strip filesystem paths and credentials and trimmed to a fixed length suitable for SSE
  - Add a worker that iterates the scheduled requests sequentially, builds pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}, calls Store.Ensure on the registry's background context (NOT the request ctx), and appends a DownloadEntryResult under the per-job mutex
  - Place a one-line `// TODO: future retry/backoff seam` comment immediately above the Ensure call to mark the deferred policy seam
  - Emit pdfdownload.entry.completed (Info on success, Warn on failure) and continue to the next entry on failure; never abort the loop
  - Observable completion: unit tests exercise (a) a successful download recording Status=success and Bytes>0, (b) a pdf.ErrFetch failure recording Status=failed/Category="fetch" and the loop continuing, and (c) cancellation of the caller-passed ctx during Schedule does not stop the worker (job runs to completion)
  - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5_
  - _Boundary: application/pdfdownload_

- [x] 2.3 Add fan-out, completion, and slow-subscriber handling
  - Maintain an events []DownloadEvent log per job (bounded by total + 1) plus a subscribers slice of buffered channels of size SubscriberBuffer
  - Append every DownloadEntryResult to the event log under the per-job mutex and fan out a Progress event to each subscriber using a non-blocking send: select { case ch <- ev: default: close(ch); drop sub; emit pdfdownload.subscriber.dropped at Warn }
  - When all entries are processed, build the final DownloadJobSnapshot, append a Summary event, set completed=true and completedAt=clock.Now(), close every remaining subscriber channel, and emit pdfdownload.job.completed with totals and duration_ms
  - Observable completion: unit tests exercise (a) a subscriber that drains receives every Progress event followed by Summary then channel close, (b) a subscriber whose channel is full beyond SubscriberBuffer is closed and dropped without blocking the worker, asserted by the worker still completing within a deadline, and (c) the job summary always emits even when every entry failed
  - _Requirements: 3.3, 4.2, 4.3, 4.4, 6.5_
  - _Boundary: application/pdfdownload_

- [x] 2.4 Implement Reader (Snapshot, Subscribe) with atomic backlog handoff
  - Implement SnapshotPDFDownloadJob: under the registry mutex, sweep expired completed jobs, then look up the job and build a snapshot from the per-job state; return paper.ErrDownloadJobUnknown for unknown or evicted ids
  - Implement SubscribePDFDownloadJob to (1) reject unknown/evicted ids, (2) snapshot the current event log into a returned backlog slice, and (3) register a fresh buffered channel into subscribers, all under the same per-job mutex in one critical section, so that the next worker append is delivered exclusively through one of backlog or live
  - Observable completion: unit tests exercise (a) status snapshot consistency with stream events for the same job, (b) atomic Subscribe under contention with -race: spawn N concurrent Subscribers while the worker emits and assert each subscriber sees every event exactly once, and (c) Snapshot/Subscribe both return ErrDownloadJobUnknown for unknown ids and after Sweep evicts a completed job
  - _Requirements: 4.1, 4.5, 5.1, 5.2, 5.3, 5.4_
  - _Boundary: application/pdfdownload_

- [x] 2.5 Implement Sweep and lazy eviction on read paths
  - Implement Sweep(ctx, now) that iterates jobs and evicts entries whose completed && now.Sub(completedAt) >= Retention; emit pdfdownload.job.evicted at Info with job_id and age_ms
  - Call sweepLocked(clock.Now()) at the top of every read path (Snapshot, Subscribe) so eviction happens lazily without a background ticker
  - Observable completion: unit tests using an injectable shared.Clock fake assert that (a) a completed job younger than Retention is retained, (b) an in-progress job is never evicted regardless of age, and (c) a completed job older than Retention is evicted on the next read and a subsequent Snapshot/Subscribe returns ErrDownloadJobUnknown
  - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_
  - _Boundary: application/pdfdownload_

## 3. Core: HTTP controllers for download endpoints

- [x] 3. PDF-download controllers

- [x] 3.1 (P) Implement the JSON status endpoint
  - Add a controller that handles GET /api/arxiv/downloads/:job_id, calls SnapshotPDFDownloadJob, marshals the snapshot through the new DTOs, and wraps paper.ErrDownloadJobUnknown into the existing *shared.HTTPError with status 404 and a stable message ("download job unknown")
  - Add full swag annotations: @Summary, @Tags, @Produce json, @Param job_id path, @Success 200 {object} JobStatusEnvelope, @Failure 404, @Security APIToken, @Router
  - Observable completion: a controller-level unit test using a fake PDFDownloadReader returns 200 with the expected JSON for a known job, returns 404 with the standard error envelope for an unknown id, and asserts response stability across in-progress vs completed jobs
  - _Requirements: 5.1, 5.2, 5.3, 5.4, 6.5_
  - _Boundary: http/controller/paper_

- [x] 3.2 (P) Implement the Server-Sent Events stream endpoint
  - Add a handler for GET /api/arxiv/downloads/:job_id/stream that calls SubscribePDFDownloadJob, returns 404 on ErrDownloadJobUnknown before sending any frame, otherwise sets text/event-stream headers and uses gin's c.Stream pattern with c.SSEvent("download.progress", payload) and c.SSEvent("download.summary", payload)
  - Replay the backlog slice first, then forward live channel events; close the response after the Summary frame; honor c.Request.Context().Done() so a client disconnect terminates the handler without affecting the underlying job; on a closed channel observed mid-stream emit a final SSE error frame
  - Add full swag annotations including produce text/event-stream and @Failure 404
  - Observable completion: a controller-level unit test using a fake PDFDownloadReader (a) returns 404 for an unknown id, (b) writes the buffered backlog before live events and closes after Summary, and (c) does not panic or leak goroutines when the request context is cancelled mid-stream
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 5.4_
  - _Boundary: http/controller/paper_

## 4. Integration: arxiv use case, response shape, routing, bootstrap

- [x] 4. Wire the new aggregate into the existing arxiv flow

- [x] 4.1 Modify the arxiv use case to schedule downloads after persistence
  - Extend NewArxivUseCase to accept a paper.PDFScheduler and store it on the use case
  - Change Fetch to return a FetchResult that pairs the existing []Result with a paper.DownloadJobSnapshot; after the persistence loop, collect IsNew==true entries, call paper.NewPDFDownloadRequests, and call SchedulePDFDownloads(ctx, requests); if Schedule returns an error, log at Error and return a zero snapshot, never failing the fetch response
  - Add the single trigger-site comment: `// Trigger location may move behind an explicit user-confirmation step in a future spec.` immediately above the Schedule call
  - Update arxiv usecase tests with the fake scheduler from task 1.6: assert exactly one Schedule call with only IsNew requests on success, no Schedule call when zero IsNew, no Schedule call when persistence fails
  - Observable completion: arxiv usecase unit tests pass with the fake scheduler asserting all three behaviors above; FetchResult callers see a non-zero snapshot only when at least one IsNew entry exists
  - _Requirements: 1.1, 1.2, 1.3, 1.4_

- [x] 4.2 Surface the initial download snapshot in the arxiv HTTP response
  - Update the arxiv controller's response builder to include the FetchResult.Job snapshot under a job field with omitempty so the JSON shape is byte-identical to today when no entry is IsNew
  - Update swag annotations to reference the extended FetchEnvelope that includes the optional download job snapshot DTO
  - Run task swag and commit the regenerated docs
  - Observable completion: a controller unit test exercises the response shape with and without IsNew entries; the response with no IsNew entries is byte-identical to the pre-spec response, and the response with IsNew entries carries job.job_id and a Pending entry per scheduled paper
  - _Requirements: 2.2_

- [x] 4.3 Register the new download route and extend route.Deps
  - Extend route.Deps with a DownloadConfig that carries paper.PDFDownloadReader; extend ArxivConfig with Scheduler paper.PDFScheduler
  - Add a new pdfdownload_route that registers GET /api/arxiv/downloads/:job_id and GET /api/arxiv/downloads/:job_id/stream under the existing /api group, building the controller locally from DownloadConfig
  - Update arxiv_route to pass d.Arxiv.Scheduler into NewArxivUseCase
  - Observable completion: a routing smoke test (via the existing tests/integration setup harness) shows both new endpoints registered and reachable, and a basic 404 path returns the standard error envelope
  - _Requirements: 2.2, 4.5, 5.3_

- [x] 4.4 Construct the registry in bootstrap and wire shutdown
  - In bootstrap, construct one *Registry via NewRegistry using pdfStore, logger, clock, env.PDFDownloadRetention, and env.PDFDownloadSubscriberBuffer; pass the same value as paper.PDFScheduler into route.ArxivConfig and as paper.PDFDownloadReader into route.DownloadConfig
  - Append the returned ShutdownFunc into the existing shutdown hooks so application stop cancels the registry's background context and waits for in-flight workers (bounded by the shutdown context)
  - Observable completion: an existing bootstrap-level test (or a new minimal one if absent) starts the app, exercises a fetch end-to-end with stub fetcher and httptest PDF server, asserts the new endpoints are registered, and asserts that triggering shutdown returns within the bounded deadline with no goroutine leaks
  - _Requirements: 3.5, 6.4_

## 5. Validation: integration and concurrency tests

- [x] 5. End-to-end validation under the integration build tag

- [x] 5.1 End-to-end happy path and failure variant
  - Add a tests/integration test that uses the existing SetupTestEnv harness and a stub arxiv fetcher returning N entries with valid PDFURLs served by an httptest.Server returning small PDF bytes
  - Drive GET /api/arxiv/fetch, capture the returned job_id from the response, open GET /downloads/{job_id}/stream and assert N download.progress events arrive followed by a download.summary event and stream close, then GET /downloads/{job_id} and assert its JSON matches the events
  - Add a failure variant where one URL returns 500 and the rest succeed; assert exactly that one entry has Status=failed/Category="fetch" and the rest are success, and that the summary counters match
  - Observable completion: integration tests pass under -race and the failure variant produces a per-entry failed/fetch outcome consistent across the stream and the status endpoint
  - _Requirements: 1.1, 2.1, 2.2, 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 5.1, 5.4_

- [x] 5.2 Concurrency and isolation
  - Add a tests/integration test that fires two GET /api/arxiv/fetch calls concurrently, asserts distinct job_id values, opens both streams concurrently, and asserts each stream receives exactly the events for its own job (no cross-talk) and both reach Summary
  - Add a separate test that opens a fetch stream, lets the client lag (slow drain), and asserts the worker still completes the job and the slow client is dropped (closed stream) without affecting the status endpoint correctness
  - Observable completion: both tests pass under -race; a final status snapshot for the slow-client job still reports the correct totals and per-entry results despite the dropped subscriber
  - _Requirements: 2.4, 4.4, 5.4_

- [x] 5.3 Retention and unknown-job behavior
  - Add a tests/integration test that completes a small job, advances the injected clock past PDFDownloadRetention, triggers a read path, and asserts both endpoints return 404 with the standard error envelope
  - Add an in-progress retention test that asserts an active job is not evicted regardless of clock advance
  - Observable completion: both tests pass; eviction logs (pdfdownload.job.evicted) are observed in the captured logger; in-progress jobs remain reachable past the retention window until completion
  - _Requirements: 6.1, 6.2, 6.3, 6.5_

## Implementation Notes

- `tests/mocks/logger.go` (`RecordingLogger.record`) is not mutex-guarded. Under heavy `go test -race -p 4 -parallel 8 -count=N` it surfaces a data race between the worker emitting the final `pdfdownload.job.completed` log and a test reading `logger.Records` after waiting for the Summary event. Required validation cadences (`-race -count=1`, `-race -count=20`) are clean. A future cleanup task may add a small sync.Mutex around append/read inside the fake. Discovered during task 2.4 review.
