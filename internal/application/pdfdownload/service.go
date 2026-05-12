// Package pdfdownload owns the PDF-download workflow end-to-end: the
// scheduler that registers jobs, the worker that fetches each PDF, the
// in-memory registry that buffers events for late subscribers, and the
// reader API consumed by HTTP controllers. The paper aggregate is read
// for paper identity only; no download-specific types live there.
//
// service.go: the Registry struct and the three public methods that
// satisfy the consumer-defined scheduler and reader interfaces.
// Schedule creates a job and launches the worker goroutine; Snapshot
// returns a point-in-time read; Subscribe atomically captures the
// backlog and registers a live channel for SSE. Also owns the jobs map,
// registryMu, bgCtx, and the Sweep/lazy-eviction logic, plus the
// NewRegistry constructor and ShutdownFunc.
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

type Options struct {
	// Retention is how long completed jobs remain readable before lazy
	// eviction on the next read path.
	Retention time.Duration
	// SubscriberBuffer is each subscriber channel's capacity; a non-
	// blocking send into it lets the worker drop slow consumers.
	SubscriberBuffer int
}

// ShutdownFunc cancels the registry's background context and waits for
// in-flight workers, bounded by the caller's ctx.
type ShutdownFunc func(ctx context.Context) error

// job is the per-job record. mu guards the mutable fields (entries,
// events, subscribers, completed, completedAt, startedAt) against the
// worker's appends; requests/id/total are set once at Schedule time and
// never mutated afterward.
type job struct {
	id       JobID
	total    int
	requests []Request

	mu          sync.Mutex
	entries     []EntryResult
	events      []Event
	subscribers []chan Event
	startedAt   time.Time
	completed   bool
	completedAt time.Time
}

// Registry is the in-memory implementation of the scheduler and reader
// surface. The same value is handed to arxiv (as a DownloadScheduler)
// and to the HTTP layer (as a PDFDownloadReader) — both interfaces are
// consumer-defined and satisfied implicitly.
//
// Lock order: registryMu (jobs map) → j.mu (per-job fields). The worker
// only ever takes j.mu, so taking registryMu and then j.mu is
// deadlock-free.
type Registry struct {
	store  pdf.Store
	logger shared.Logger
	clock  shared.Clock
	opts   Options

	// bgCtx hosts workers so a fetch-request cancellation never
	// propagates into a running download (R3.5).
	bgCtx        context.Context
	bgCancel     context.CancelFunc
	shutdownOnce sync.Once
	workers      sync.WaitGroup

	registryMu sync.Mutex
	jobs       map[JobID]*job
}

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
		jobs:     make(map[JobID]*job),
	}
	shutdown := func(ctx context.Context) error {
		r.shutdownOnce.Do(func() {
			r.bgCancel()
			// Workers ignore ctx (R3.5), so the deadline only releases
			// the shutdown caller; it does not interrupt downloads.
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

// Schedule registers a new job atomically and returns its initial
// snapshot. The passed ctx is intentionally not consulted — a client
// disconnect between persistence and snapshot return must not cancel
// the just-registered job (R3.5).
func (r *Registry) Schedule(ctx context.Context, requests []Request) (JobSnapshot, error) {
	if len(requests) == 0 {
		return JobSnapshot{}, nil
	}

	id := JobID(uuid.NewString())
	entries := make([]EntryResult, 0, len(requests))
	paperIDs := make([]string, 0, len(requests))
	for _, req := range requests {
		entries = append(entries, EntryResult{
			PaperID: req.PaperID,
			Status:  StatusPending,
		})
		paperIDs = append(paperIDs, formatPaperID(req.PaperID))
	}

	jobRequests := make([]Request, len(requests))
	copy(jobRequests, requests)

	j := &job{
		id:       id,
		total:    len(requests),
		requests: jobRequests,
		entries:  entries,
		// Events log capped at total+1 (one Progress per entry plus
		// the terminal Summary) — no more appends ever happen after.
		events: make([]Event, 0, len(requests)+1),
	}

	r.registryMu.Lock()
	r.jobs[id] = j
	r.registryMu.Unlock()

	// Build the initial snapshot outside the registry mutex so we don't
	// hold the global lock during the O(N) entries copy. The worker
	// hasn't started yet, so reading j.entries (all Pending) is race-free.
	snap := snapshotForJob(j)

	r.logger.InfoContext(ctx, "pdfdownload.job.scheduled",
		"job_id", string(id),
		"total", len(requests),
		"paper_ids", paperIDs,
	)

	r.workers.Add(1)
	go r.runJob(j)

	return snap, nil
}

// Snapshot returns ErrJobUnknown for unknown or evicted jobs, otherwise
// the current per-entry results and totals.
func (r *Registry) Snapshot(ctx context.Context, id JobID) (JobSnapshot, error) {
	r.registryMu.Lock()
	evicted := r.sweepLocked(r.clock.Now())
	j, ok := r.jobs[id]
	r.registryMu.Unlock()
	r.logEvictions(ctx, evicted)
	if !ok {
		return JobSnapshot{}, ErrJobUnknown
	}

	return snapshotForJob(j), nil
}

// Subscribe captures the current event log into backlog and registers a
// buffered live channel atomically under j.mu. Once it returns, every
// event produced by the worker arrives via exactly one of backlog or
// live — never both, never neither (R4.1, R5.4). A job that already
// completed gets a fully-populated backlog and a pre-closed live channel.
func (r *Registry) Subscribe(ctx context.Context, id JobID) ([]Event, <-chan Event, error) {
	r.registryMu.Lock()
	evicted := r.sweepLocked(r.clock.Now())
	j, ok := r.jobs[id]
	r.registryMu.Unlock()
	r.logEvictions(ctx, evicted)
	if !ok {
		return nil, nil, ErrJobUnknown
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	backlog := make([]Event, len(j.events))
	copy(backlog, j.events)

	live := make(chan Event, r.opts.SubscriberBuffer)
	if j.completed {
		// No more events will fire; close immediately. Don't append to
		// j.subscribers — finishJob already cleared the slice.
		close(live)
		return backlog, live, nil
	}

	j.subscribers = append(j.subscribers, live)
	return backlog, live, nil
}

// Sweep evicts every completed job whose age has reached Retention.
// Exposed for deterministic test control; production paths sweep lazily
// on read.
func (r *Registry) Sweep(ctx context.Context, now time.Time) {
	r.registryMu.Lock()
	evicted := r.sweepLocked(now)
	r.registryMu.Unlock()
	r.logEvictions(ctx, evicted)
}

// evictionLog records one eviction for deferred logging outside the
// registry mutex.
type evictionLog struct {
	id  JobID
	age time.Duration
}

// sweepLocked deletes completed jobs whose age >= Retention and returns
// the eviction records. Caller must hold r.registryMu and is responsible
// for emitting the logs after releasing it.
func (r *Registry) sweepLocked(now time.Time) []evictionLog {
	var evicted []evictionLog
	for id, j := range r.jobs {
		j.mu.Lock()
		completed := j.completed
		completedAt := j.completedAt
		j.mu.Unlock()
		if !completed {
			continue
		}
		age := now.Sub(completedAt)
		if age < r.opts.Retention {
			continue
		}
		delete(r.jobs, id)
		evicted = append(evicted, evictionLog{id: id, age: age})
	}
	return evicted
}

func (r *Registry) logEvictions(ctx context.Context, evicted []evictionLog) {
	for _, e := range evicted {
		r.logger.InfoContext(ctx, "pdfdownload.job.evicted",
			"job_id", string(e.id),
			"age_ms", e.age.Milliseconds(),
		)
	}
}

// snapshotForJob projects j into a snapshot reflecting current state.
// Acquires j.mu while reading; safe to call while the worker is
// appending.
func snapshotForJob(j *job) JobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	return buildSnapshotLocked(j, j.completed, j.completedAt)
}

// formatPaperID renders id as `<source>:<sourceid>[<version>]` for logs.
func formatPaperID(id paper.ID) string {
	if id.Version == "" {
		return id.Source + ":" + id.SourceID
	}
	return id.Source + ":" + id.SourceID + id.Version
}
