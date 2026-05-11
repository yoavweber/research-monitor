// Package pdfdownload owns the in-memory registry that implements both
// paper.PDFScheduler (write side) and paper.PDFDownloadReader (read side).
//
// State boundaries:
//   - jobs map and registry-owned background context live in this package.
//   - Schedule mutates state under a single mutex without consulting the
//     caller's ctx (see design.md "Schedule contract is non-cancellable").
//   - The registry depends only on domain ports: pdf.Store, shared.Logger,
//     shared.Clock. No infrastructure imports.
package pdfdownload

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Compile-time conformance: a *Registry value satisfies both the
// write-side scheduler port and the read-side reader port. This is the
// architectural commitment from design.md "Application —
// application/pdfdownload": one struct, two ports, never duplicated.
var (
	_ paper.PDFScheduler      = (*Registry)(nil)
	_ paper.PDFDownloadReader = (*Registry)(nil)
)

// Options configures a Registry. The bootstrap layer chooses defaults;
// the registry itself does not substitute them. Fields stay reachable
// for tasks 2.3 (SubscriberBuffer) and 2.5 (Retention) without changing
// the constructor signature.
type Options struct {
	// Retention is how long completed jobs are kept before lazy
	// eviction on the next read path. Used by task 2.5.
	Retention time.Duration

	// SubscriberBuffer sizes each subscriber's channel. A non-blocking
	// send into this buffer is what lets the worker drop slow consumers
	// without stalling. Used by tasks 2.3 and 2.4.
	SubscriberBuffer int
}

// ShutdownFunc tears down the registry. It cancels the registry-owned
// background context (so any worker goroutines spawned by future tasks
// stop progressing) and bounds the wait by ctx. For 2.1 there are no
// workers yet, so it cancels and returns nil.
type ShutdownFunc func(ctx context.Context) error

// job is the internal per-job record. Task 2.1 populated id, total, and
// entries (all Pending). Task 2.2 added the request slice the worker
// iterates and a completion flag flipped after the worker drains the
// request loop. Task 2.3 added the bounded event log (total+1: one
// Progress per entry plus the terminal Summary) and the subscribers
// slice that the worker fans out to under j.mu.
type job struct {
	id       paper.DownloadJobID
	total    int
	requests []paper.PDFDownloadRequest

	// mu guards entries, events, subscribers, completed, startedAt, and
	// completedAt. The registry mutex serialises jobs map lookups; this
	// mutex serialises per-entry appends and fan-out so a slow consumer
	// cannot block other jobs via the global registry lock.
	mu          sync.Mutex
	entries     []paper.DownloadEntryResult
	events      []paper.DownloadEvent
	subscribers []chan paper.DownloadEvent
	startedAt   time.Time
	completed   bool
	completedAt time.Time
}

// Registry is the in-memory implementation of paper.PDFScheduler and
// paper.PDFDownloadReader. The same value satisfies both ports; the
// arxiv use case sees only the scheduler side, the download controller
// sees only the reader side.
type Registry struct {
	store  pdf.Store
	logger shared.Logger
	clock  shared.Clock
	opts   Options

	// bgCtx is the registry-owned background context. Workers (added in
	// task 2.2) will derive their per-job context from this so a fetch
	// request cancellation never propagates into a running download.
	// Cancellation flows in only via ShutdownFunc.
	bgCtx    context.Context
	bgCancel context.CancelFunc

	// shutdownOnce makes ShutdownFunc safe to call repeatedly.
	shutdownOnce sync.Once

	// workers tracks in-flight job goroutines so ShutdownFunc can wait
	// for them to finish (task 2.2 launches the workers; later tasks
	// may extend the drain semantics in coordination with bgCancel).
	workers sync.WaitGroup

	// registryMu guards jobs. A single mutex is the design's invariant:
	// Schedule mutates atomically, and read paths (added in 2.4) sweep
	// then look up, all under this lock.
	registryMu sync.Mutex
	jobs       map[paper.DownloadJobID]*job
}

// NewRegistry constructs a Registry plus its ShutdownFunc. The bgCtx
// is created here once; tasks 2.2/2.3 will use it to host workers.
func NewRegistry(
	store pdf.Store,
	logger shared.Logger,
	clock shared.Clock,
	opts Options,
) (*Registry, ShutdownFunc) {
	bgCtx, cancel := context.WithCancel(context.Background())
	r := &Registry{
		store:    store,
		logger:   logger,
		clock:    clock,
		opts:     opts,
		bgCtx:    bgCtx,
		bgCancel: cancel,
		jobs:     make(map[paper.DownloadJobID]*job),
	}
	shutdown := func(ctx context.Context) error {
		r.shutdownOnce.Do(func() {
			r.bgCancel()
			// Bound the drain by ctx so a shutdown caller can cap how
			// long it is willing to wait for in-flight workers. v1
			// workers ignore ctx (R3.5) so we cannot interrupt them;
			// the deadline still releases the caller.
			done := make(chan struct{})
			go func() {
				r.workers.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-ctx.Done():
			}
		})
		return nil
	}
	return r, shutdown
}

// SchedulePDFDownloads registers a new job atomically and returns its
// initial snapshot. The passed ctx is intentionally not consulted: a
// client disconnecting between the persistence loop and the snapshot
// return must not cancel the just-registered job (see design.md
// "Schedule contract is non-cancellable", R3.5).
func (r *Registry) SchedulePDFDownloads(ctx context.Context, requests []paper.PDFDownloadRequest) (paper.DownloadJobSnapshot, error) {
	if len(requests) == 0 {
		return paper.DownloadJobSnapshot{}, nil
	}

	id := paper.DownloadJobID(uuid.NewString())
	entries := make([]paper.DownloadEntryResult, 0, len(requests))
	paperIDs := make([]string, 0, len(requests))
	for _, req := range requests {
		entries = append(entries, paper.DownloadEntryResult{
			PaperID: req.PaperID,
			Status:  paper.DownloadStatusPending,
		})
		paperIDs = append(paperIDs, formatPaperID(req.PaperID))
	}

	// Copy requests onto the job so the worker iterates a stable slice
	// independent of the caller's backing array. The worker reads
	// requests without taking j.mu (it is set once here and never
	// mutated again).
	jobRequests := make([]paper.PDFDownloadRequest, len(requests))
	copy(jobRequests, requests)

	j := &job{
		id:       id,
		total:    len(requests),
		requests: jobRequests,
		entries:  entries,
		// Capacity is exactly total+1: one Progress per entry plus the
		// terminal Summary. No more appends ever happen after Summary, so
		// the slice never grows past this bound (R4.* invariant).
		events: make([]paper.DownloadEvent, 0, len(requests)+1),
	}

	r.registryMu.Lock()
	r.jobs[id] = j
	// Snapshot under the registry mutex so the returned value is
	// derived from the same state we just installed; entries is copied
	// to insulate the caller from any later in-place mutation by the
	// worker.
	snap := snapshotForJob(j)
	r.registryMu.Unlock()

	r.logger.InfoContext(ctx, "pdfdownload.job.scheduled",
		"job_id", string(id),
		"total", len(requests),
		"paper_ids", paperIDs,
	)

	r.workers.Add(1)
	go r.runJob(j)

	return snap, nil
}

// SnapshotPDFDownloadJob returns the current per-entry results, totals,
// and completion flag for jobID. Returns paper.ErrDownloadJobUnknown
// when the id is unknown or has been evicted (R5.1, R5.3, R6.3).
//
// Locking discipline: the registry mutex serialises the map lookup; the
// per-job mutex serialises the read of entries/completed/completedAt
// against the worker's appends. We release the registry mutex before
// acquiring the per-job mutex to keep contention on the global lock
// short. This is safe because the registry never removes a job out from
// under a read in task 2.4 — Sweep (task 2.5) is the only path that
// would, and it has not been wired in yet (see sweepLocked placeholder).
func (r *Registry) SnapshotPDFDownloadJob(_ context.Context, id paper.DownloadJobID) (paper.DownloadJobSnapshot, error) {
	r.registryMu.Lock()
	// sweepLocked(r.clock.Now()) — added in task 2.5
	j, ok := r.jobs[id]
	r.registryMu.Unlock()
	if !ok {
		return paper.DownloadJobSnapshot{}, paper.ErrDownloadJobUnknown
	}

	return snapshotForJob(j), nil
}

// SubscribePDFDownloadJob captures the current event log into a fresh
// backlog slice and registers a buffered live channel atomically under
// the per-job mutex (design.md "Subscribe is atomic", R4.1, R5.4).
//
// Guarantee: once this returns, every paper.DownloadEvent produced by
// the worker is delivered through exactly one of backlog or live —
// never both, never neither. The worker's appendAndFanOut takes only
// the per-job mutex, so an event either lands in the events log before
// our copy (and thus appears in backlog) or lands after we install our
// channel into subscribers (and thus arrives on live).
//
// Post-completion case: if the worker has already finished, j.events
// already holds the full log including the terminal Summary. We copy
// it into backlog and return a pre-closed live channel; appending to
// j.subscribers would be pointless because no further appends will
// happen and finishJob has already closed every prior subscriber.
//
// Locking: registry mutex first to look up the job, then release it
// before taking the per-job mutex. The worker never takes the registry
// mutex, so holding both simultaneously is also deadlock-free, but
// releasing first keeps the global lock available for other Schedule
// calls during a slow consumer's setup. Job pointers cannot be
// invalidated between the two acquires because Sweep is not wired
// (task 2.5).
func (r *Registry) SubscribePDFDownloadJob(_ context.Context, id paper.DownloadJobID) ([]paper.DownloadEvent, <-chan paper.DownloadEvent, error) {
	r.registryMu.Lock()
	// sweepLocked(r.clock.Now()) — added in task 2.5
	j, ok := r.jobs[id]
	r.registryMu.Unlock()
	if !ok {
		return nil, nil, paper.ErrDownloadJobUnknown
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	backlog := make([]paper.DownloadEvent, len(j.events))
	copy(backlog, j.events)

	live := make(chan paper.DownloadEvent, r.opts.SubscriberBuffer)
	if j.completed {
		// No more events will ever fire; close immediately so the
		// caller's drain loop terminates. Do NOT append to j.subscribers
		// — finishJob already closed every prior subscriber and cleared
		// the slice.
		close(live)
		return backlog, live, nil
	}

	j.subscribers = append(j.subscribers, live)
	return backlog, live, nil
}

// snapshotForJob projects a job into the public snapshot shape. Lives
// here (not on *job) so the projection is testable from package code
// and reused by both SchedulePDFDownloads (initial all-Pending case)
// and SnapshotPDFDownloadJob (mid-flight or post-completion). Acquires
// j.mu while reading so it is safe to call while the worker is
// appending. Succeeded/Failed count the entries whose Status has been
// flipped past Pending; on the initial snapshot both are zero.
func snapshotForJob(j *job) paper.DownloadJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	var succeeded, failed int
	for _, e := range j.entries {
		switch e.Status {
		case paper.DownloadStatusSuccess:
			succeeded++
		case paper.DownloadStatusFailed:
			failed++
		}
	}
	entries := make([]paper.DownloadEntryResult, len(j.entries))
	copy(entries, j.entries)
	return paper.DownloadJobSnapshot{
		JobID:       j.id,
		Total:       j.total,
		Succeeded:   succeeded,
		Failed:      failed,
		Completed:   j.completed,
		CompletedAt: j.completedAt,
		Entries:     entries,
	}
}

// formatPaperID is the structured-log shape for a paper.ID. Mirrors
// design.md's intended (ID).String() format ("<source>:<sourceid>[v<version>]")
// without depending on a method that lives outside this task's
// boundary; the spec adds (ID).String() in a follow-up paper-package
// task and call sites here will switch over then.
func formatPaperID(id paper.ID) string {
	if id.Version == "" {
		return id.Source + ":" + id.SourceID
	}
	return id.Source + ":" + id.SourceID + id.Version
}
