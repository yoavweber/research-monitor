package extraction

import (
	"context"
	"fmt"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Worker is the background drainer for pending extraction rows. Bootstrap
// constructs it after RecoverRunningOnStartup, calls Start once, and Stops it
// after the app context is cancelled and before the DB closes. Worker is
// concrete (not behind an interface) because Start / Stop are lifecycle
// methods specific to this in-process implementation, not part of any
// cross-layer port.
type Worker struct {
	repo      extraction.Repository
	useCase   extraction.UseCase
	logger    shared.Logger
	clock     shared.Clock
	wakeCh    <-chan struct{}
	jobExpiry time.Duration

	// done is closed when the goroutine exits so Stop can block on it.
	done chan struct{}
}

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

func (w *Worker) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop blocks until the wake-loop goroutine has returned. Caller is responsible
// for cancelling the worker's ctx beforehand — that is what triggers the exit.
func (w *Worker) Stop() {
	<-w.done
}

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

func (w *Worker) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		peeked, ok, err := w.repo.PeekNextPending(ctx)
		if err != nil {
			w.logger.WarnContext(ctx, "extraction.worker.peek_failed", "error", err.Error())
			return
		}
		if !ok {
			return
		}

		// Expiry is decided while the row is still pending so an expired row
		// transitions straight to failed without ever entering running.
		if w.clock.Now().After(peeked.CreatedAt.Add(w.jobExpiry)) {
			w.logger.WarnContext(ctx, "extraction.worker.expired",
				"id", peeked.ID,
				"created_at", peeked.CreatedAt,
				"job_expiry", w.jobExpiry,
			)
			message := fmt.Sprintf("created_at %s older than job_expiry %s", peeked.CreatedAt, w.jobExpiry)
			if mfErr := w.repo.MarkFailed(ctx, peeked.ID, extraction.FailureReasonExpired, message); mfErr != nil {
				w.logger.WarnContext(ctx, "extraction.worker.expire_mark_failed",
					"id", peeked.ID,
					"error", mfErr.Error(),
				)
			}
			continue
		}

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

		// The peeked snapshot is stale once we've claimed; reflect post-claim
		// state on the local copy passed to Process.
		peeked.Status = extraction.JobStatusRunning
		if procErr := w.useCase.Process(ctx, *peeked); procErr != nil {
			w.logger.WarnContext(ctx, "extraction.worker.process_error",
				"id", peeked.ID,
				"error", procErr.Error(),
			)
			// Process returns ctx.Err() without writing on cancellation; the
			// row stays in running for next-boot recovery. Break the drain so
			// the outer select observes ctx.Done().
			if ctx.Err() != nil {
				return
			}
		}
	}
}
