// Package extraction's worker.go owns the background goroutine that drains
// pending extraction rows. The Worker receives wake signals from the use
// case's Submit path on a buffered channel, peeks the next pending row,
// evaluates the pickup-time expiry predicate before claiming, claims, and
// hands the running row to UseCase.Process.
//
// The pickup-time expiry check is the resolution to design Critical Issue 2:
// an expired row is MarkFailed-from-pending and the Extractor is never
// invoked. The graceful-shutdown semantics — leaving a mid-flight row in
// running for the next-boot RecoverRunningOnStartup pass — is the resolution
// to Critical Issue 1.
package extraction

import (
	"context"
	"fmt"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Worker is the background drainer. It is constructed by bootstrap, started
// once after RecoverRunningOnStartup has flipped any orphaned running rows,
// and stopped (after appCtx is cancelled) before the DB handle is closed.
//
// Worker is a concrete struct rather than an interface because Start / Stop
// are lifecycle methods specific to this in-process implementation, not part
// of any port that is mocked across layers.
type Worker struct {
	repo      extraction.Repository
	useCase   extraction.UseCase
	logger    shared.Logger
	clock     shared.Clock
	wakeCh    <-chan struct{}
	jobExpiry time.Duration

	// done is closed when the goroutine exits so Stop() can block on it.
	done chan struct{}
}

// NewWorker wires the Worker dependencies. wakeCh is the receive end of the
// buffered channel whose send end the use case writes to from Submit.
// jobExpiry is the configured maximum age between Submit and pickup; rows
// older than that at peek time are failed without ever entering running.
func NewWorker(
	repo extraction.Repository,
	useCase extraction.UseCase,
	logger shared.Logger,
	clock shared.Clock,
	wakeCh <-chan struct{},
	jobExpiry time.Duration,
) *Worker {
	return &Worker{
		repo:      repo,
		useCase:   useCase,
		logger:    logger,
		clock:     clock,
		wakeCh:    wakeCh,
		jobExpiry: jobExpiry,
		done:      make(chan struct{}),
	}
}

// Start launches the wake-loop goroutine and returns immediately. Calling
// Start more than once on the same Worker is undefined; bootstrap calls it
// exactly once.
func (w *Worker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop blocks until the wake-loop goroutine has returned. Bootstrap calls
// Stop after cancelling the app context and before closing the DB handle.
func (w *Worker) Stop() {
	<-w.done
}

// run is the long-lived goroutine body. The outer select waits for either
// ctx cancellation or a wake signal; on wake, the inner drain loop pulls
// every pending row that's currently in the queue. Both loops respect ctx
// cancellation and break promptly so Stop returns without lingering work.
func (w *Worker) run(ctx context.Context) {
	defer close(w.done)
	w.logger.InfoContext(ctx, "extraction.worker.start")

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "extraction.worker.stop")
			return
		case <-w.wakeCh:
		}

		w.drain(ctx)
	}
}

// drain processes rows until the queue is empty, an error breaks the loop,
// or ctx is cancelled. Each branch logs its own structured event so a
// transient repo failure does not crash the goroutine — the worker simply
// waits for the next wake signal.
func (w *Worker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		peeked, ok, err := w.repo.PeekNextPending(ctx)
		if err != nil {
			w.logger.WarnContext(ctx, "extraction.worker.peek_failed",
				"error", err.Error(),
			)
			return
		}
		if !ok {
			return
		}

		// Pickup-time expiry: evaluated while the row is still pending so an
		// expired row is failed without ever entering running. Only after
		// this gate do we claim.
		if w.clock.Now().After(peeked.CreatedAt.Add(w.jobExpiry)) {
			w.logger.WarnContext(ctx, "extraction.worker.expired",
				"id", peeked.ID,
				"created_at", peeked.CreatedAt,
				"job_expiry", w.jobExpiry,
			)
			message := fmt.Sprintf(
				"created_at %s older than job_expiry %s",
				peeked.CreatedAt, w.jobExpiry,
			)
			if mfErr := w.repo.MarkFailed(ctx, peeked.ID, extraction.FailureReasonExpired, message); mfErr != nil {
				w.logger.WarnContext(ctx, "extraction.worker.expire_mark_failed",
					"id", peeked.ID,
					"error", mfErr.Error(),
				)
			}
			continue
		}

		// Claim transitions pending -> running. A non-nil error here means
		// either ctx was cancelled or another writer claimed the row (the
		// multi-worker future); both cases are recoverable — we log and
		// continue draining.
		if claimErr := w.repo.ClaimPending(ctx, peeked.ID); claimErr != nil {
			w.logger.WarnContext(ctx, "extraction.worker.claim_failed",
				"id", peeked.ID,
				"error", claimErr.Error(),
			)
			if ctx.Err() != nil {
				return
			}
			continue
		}

		// Process expects the row in running. The peeked snapshot is stale
		// post-claim; mutate the local copy's Status so the value the use
		// case sees reflects post-claim state. Process itself does not key
		// off Status (it is invoked only after the worker has claimed).
		peeked.Status = extraction.JobStatusRunning
		if procErr := w.useCase.Process(ctx, *peeked); procErr != nil {
			w.logger.WarnContext(ctx, "extraction.worker.process_error",
				"id", peeked.ID,
				"error", procErr.Error(),
			)
			// On ctx cancellation Process returns ctx.Err() without writing;
			// the row is left in running for next-boot recovery. Break out
			// of the drain so the outer select observes ctx.Done().
			if ctx.Err() != nil {
				return
			}
		}
	}
}
