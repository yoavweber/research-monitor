package extraction_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	appextraction "github.com/yoavweber/research-monitor/backend/internal/application/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	extractionrepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/extraction"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
	"github.com/yoavweber/research-monitor/backend/tests/testdb"
)

// workerFixture wires the dependencies the Worker needs against a real
// GORM-over-SQLite repository, the existing fake Extractor, a recording
// logger, and a ChannelNotifier of caller-chosen buffer capacity. wakeCap
// must be ≥1 so each test's first Notify after Start always lands.
type workerFixture struct {
	db        *gorm.DB
	repo      extraction.Repository
	uc        extraction.UseCase
	extractor *mocks.ExtractorFake
	logger    *mocks.RecordingLogger
	clock     *mocks.MovableClock
	notifier  *appextraction.ChannelNotifier
	worker    *appextraction.Worker
}

func newWorkerFixture(t *testing.T, wakeCap int, jobExpiry time.Duration) *workerFixture {
	t.Helper()
	db := testdb.New(t)
	repo := extractionrepo.NewRepository(db)
	extractor := &mocks.ExtractorFake{}
	logger := &mocks.RecordingLogger{}
	clock := mocks.NewMovableClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	notifier := appextraction.NewChannelNotifier(wakeCap)

	uc := appextraction.NewExtractionUseCase(repo, extractor, logger, clock, notifier, 100_000)
	worker := appextraction.NewWorker(repo, uc, logger, clock, notifier.C(), jobExpiry)

	return &workerFixture{
		db:        db,
		repo:      repo,
		uc:        uc,
		extractor: extractor,
		logger:    logger,
		clock:     clock,
		notifier:  notifier,
		worker:    worker,
	}
}

// waitFor polls f every 10ms until it returns true or timeout fires. It is
// the test-side synchronization primitive for the Worker's asynchronous
// state transitions.
func waitFor(t *testing.T, f func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !f() {
		t.Fatalf("waitFor timed out after %s: %s", timeout, msg)
	}
}

func TestWorker(t *testing.T) {
	t.Parallel()

	t.Run("peek then claim drives a non-expired row to done in a single drain pass", func(t *testing.T) {
		t.Parallel()

		fx := newWorkerFixture(t, 1, time.Hour)
		fx.extractor.Output = extraction.ExtractOutput{Markdown: "# Title\n\nbody"}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		id, _, err := fx.repo.Upsert(ctx, extraction.RequestPayload{
			SourceType: "paper",
			SourceID:   "src-happy",
			PDFPath:    "/tmp/happy.pdf",
		})
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}
		// Clock is well within the expiry window for the just-created row.
		fx.clock.Set(time.Now().UTC())

		fx.worker.Start(ctx)
		fx.notifier.Notify(ctx)

		waitFor(t, func() bool {
			row, err := fx.repo.FindByID(ctx, id)
			return err == nil && row.Status == extraction.JobStatusDone
		}, 2*time.Second, "row never reached done")

		cancel()
		fx.worker.Stop()

		row, err := fx.repo.FindByID(context.Background(), id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if row.Status != extraction.JobStatusDone {
			t.Errorf("status = %q, want done", row.Status)
		}
		if got := len(fx.extractor.RecordedCalls()); got != 1 {
			t.Errorf("Extractor calls = %d, want 1", got)
		}
	})

	t.Run("expired row is failed straight from pending without invoking the extractor", func(t *testing.T) {
		t.Parallel()

		jobExpiry := 10 * time.Minute
		fx := newWorkerFixture(t, 1, jobExpiry)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		id, _, err := fx.repo.Upsert(ctx, extraction.RequestPayload{
			SourceType: "paper",
			SourceID:   "src-expired",
			PDFPath:    "/tmp/expired.pdf",
		})
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		// Push the row's created_at backwards so it falls outside the expiry
		// window relative to the frozen clock. Direct GORM update bypasses
		// the repo's auto-managed timestamps, which is what we need for
		// determinism.
		now := fx.clock.Now()
		oldCreatedAt := now.Add(-2 * jobExpiry)
		if err := fx.db.Model(&extractionrepo.Extraction{}).
			Where("id = ?", id).
			Update("created_at", oldCreatedAt).Error; err != nil {
			t.Fatalf("backdate created_at: %v", err)
		}

		fx.worker.Start(ctx)
		fx.notifier.Notify(ctx)

		waitFor(t, func() bool {
			row, err := fx.repo.FindByID(ctx, id)
			return err == nil && row.Status == extraction.JobStatusFailed
		}, 2*time.Second, "row never reached failed")

		cancel()
		fx.worker.Stop()

		row, err := fx.repo.FindByID(context.Background(), id)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if row.Status != extraction.JobStatusFailed {
			t.Fatalf("status = %q, want failed", row.Status)
		}
		if row.Failure == nil || row.Failure.Reason != extraction.FailureReasonExpired {
			t.Fatalf("failure = %+v, want reason=expired", row.Failure)
		}
		if got := len(fx.extractor.RecordedCalls()); got != 0 {
			t.Errorf("Extractor calls = %d, want 0 for expired row", got)
		}
		// The MarkFailed message must carry both the created_at and the
		// job_expiry value so operators can reconstruct the expiry decision
		// from logs alone.
		msg := row.Failure.Message
		if !strings.Contains(msg, "created_at") || !strings.Contains(msg, "job_expiry") {
			t.Errorf("failure message = %q, want it to mention both created_at and job_expiry", msg)
		}
		if !strings.Contains(msg, jobExpiry.String()) {
			t.Errorf("failure message = %q, want it to contain job_expiry value %q", msg, jobExpiry.String())
		}
		// The created_at year (2025) is part of the row's timestamp as it
		// flows through the message via fmt.Sprintf("%s") — checking the
		// year keeps the assertion stable against minor format changes.
		if !strings.Contains(msg, "2024") {
			t.Errorf("failure message = %q, want it to contain backdated created_at year 2024", msg)
		}
	})

	t.Run("non-blocking submit under bursty load drops excess wakes and still drains every row", func(t *testing.T) {
		t.Parallel()

		fx := newWorkerFixture(t, 2, time.Hour)
		release := make(chan struct{})
		fx.extractor.BlockUntil = release
		fx.extractor.Output = extraction.ExtractOutput{Markdown: "# Title\n\nbody"}
		fx.clock.Set(time.Now().UTC())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Seed one row plus its wake so the worker enters Process and pins
		// itself on BlockUntil before the burst Submits arrive.
		seedID, _, err := fx.repo.Upsert(ctx, extraction.RequestPayload{
			SourceType: "paper",
			SourceID:   "src-seed",
			PDFPath:    "/tmp/seed.pdf",
		})
		if err != nil {
			t.Fatalf("Upsert seed: %v", err)
		}

		fx.worker.Start(ctx)
		fx.notifier.Notify(ctx)

		waitFor(t, func() bool {
			return len(fx.extractor.RecordedCalls()) == 1
		}, 2*time.Second, "worker never entered Process for the seed row")

		// Burst of 5 Submits: each one tries a non-blocking send into a
		// channel of cap 2 that already has 0 free slots until the worker
		// drains. None of the Submits must block; the wakeCh's default
		// branch is the safety valve.
		ids := []string{seedID}
		for i := 0; i < 5; i++ {
			done := make(chan struct{})
			payload := extraction.RequestPayload{
				SourceType: "paper",
				SourceID:   "burst-" + strconv.Itoa(i),
				PDFPath:    "/tmp/burst.pdf",
			}
			var (
				res    extraction.SubmitResult
				subErr error
			)
			go func() {
				defer close(done)
				res, subErr = fx.uc.Submit(ctx, payload)
			}()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				t.Fatalf("Submit %d blocked — wake channel send was not non-blocking", i)
			}
			if subErr != nil {
				t.Fatalf("Submit %d: %v", i, subErr)
			}
			ids = append(ids, res.ID)
		}

		// Release the seed Process so the worker can drain the rest.
		close(release)

		waitFor(t, func() bool {
			pending, err := fx.repo.ListPendingIDs(context.Background())
			return err == nil && len(pending) == 0
		}, 5*time.Second, "worker did not drain every committed row")

		cancel()
		fx.worker.Stop()

		for _, id := range ids {
			row, err := fx.repo.FindByID(context.Background(), id)
			if err != nil {
				t.Fatalf("FindByID %s: %v", id, err)
			}
			if row.Status != extraction.JobStatusDone {
				t.Errorf("row %s status = %q, want done", id, row.Status)
			}
		}
	})

	t.Run("ctx cancellation mid-process leaves the row in running and recovery flips it to process_restart", func(t *testing.T) {
		t.Parallel()

		fx := newWorkerFixture(t, 1, time.Hour)
		release := make(chan struct{})
		defer func() {
			// Defensive: if we somehow exit before closing release, unblock
			// any lingering Extract call to avoid leaks across t.Parallel.
			select {
			case <-release:
			default:
				close(release)
			}
		}()
		fx.extractor.BlockUntil = release
		fx.extractor.Output = extraction.ExtractOutput{Markdown: "# Title\n\nbody"}
		fx.clock.Set(time.Now().UTC())

		ctx, cancel := context.WithCancel(context.Background())

		id, _, err := fx.repo.Upsert(ctx, extraction.RequestPayload{
			SourceType: "paper",
			SourceID:   "src-shutdown",
			PDFPath:    "/tmp/shutdown.pdf",
		})
		if err != nil {
			t.Fatalf("Upsert: %v", err)
		}

		fx.worker.Start(ctx)
		fx.notifier.Notify(ctx)

		waitFor(t, func() bool {
			return len(fx.extractor.RecordedCalls()) == 1
		}, 2*time.Second, "worker never entered Process")

		// Worker is now blocked inside Extract on release. Cancel — the
		// fake Extract returns ctx.Err(); Process sees ctx.Err() and
		// returns without writing; the worker's drain breaks; the wake-
		// loop's outer select catches ctx.Done() and the goroutine exits.
		cancel()
		fx.worker.Stop()

		row, err := fx.repo.FindByID(context.Background(), id)
		if err != nil {
			t.Fatalf("FindByID after Stop: %v", err)
		}
		if row.Status != extraction.JobStatusRunning {
			t.Fatalf("status after shutdown = %q, want running (no MarkFailed write during shutdown)", row.Status)
		}

		// Next-boot recovery: RecoverRunningOnStartup flips orphaned running
		// rows to failed: process_restart. This is the partner half of the
		// shutdown invariant — the worker leaves the row, the next boot
		// cleans it up.
		recovered, err := fx.repo.RecoverRunningOnStartup(context.Background())
		if err != nil {
			t.Fatalf("RecoverRunningOnStartup: %v", err)
		}
		if recovered != 1 {
			t.Errorf("recovered = %d, want 1", recovered)
		}

		row, err = fx.repo.FindByID(context.Background(), id)
		if err != nil {
			t.Fatalf("FindByID after recovery: %v", err)
		}
		if row.Status != extraction.JobStatusFailed {
			t.Fatalf("status after recovery = %q, want failed", row.Status)
		}
		if row.Failure == nil || row.Failure.Reason != extraction.FailureReasonProcessRestart {
			t.Errorf("failure = %+v, want reason=process_restart", row.Failure)
		}
	})
}
