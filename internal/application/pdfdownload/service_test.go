package pdfdownload_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// newRegistryForTest builds a Registry wired with hand-rolled fakes from
// tests/mocks/. Schedule itself does not consult store/clock, but the
// constructor accepts both to match the construction signature documented in
// design.md.
func newRegistryForTest(t *testing.T) (*pdfdownload.Registry, *mocks.RecordingLogger, pdfdownload.ShutdownFunc) {
	t.Helper()

	store := mocks.NewPDFStore(t.TempDir())
	logger := &mocks.RecordingLogger{}
	clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

	reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
		Retention:        5 * time.Minute,
		SubscriberBuffer: 32,
	})
	t.Cleanup(func() {
		_ = shutdown(context.Background())
	})
	return reg, logger, shutdown
}

// formatPaperID mirrors the design's intended paper.ID string form:
// "<source>:<sourceid>[v<version>]". Inlined here so the test asserts the
// exact wire shape without depending on a paper.ID method that has not been
// added to the paper package yet (the paper aggregate is outside this task's
// boundary).
func formatPaperID(id paper.ID) string {
	if id.Version == "" {
		return id.Source + ":" + id.SourceID
	}
	return id.Source + ":" + id.SourceID + id.Version
}

// gatedStore wraps a mocks.PDFStore with a release channel: Ensure blocks
// until the channel is closed (or the caller's ctx is cancelled). It lets
// fan-out tests attach a subscriber after Schedule but before the worker
// emits any event, without depending on the registry's internal
// sequencing.
type gatedStore struct {
	inner   pdf.Store
	release <-chan struct{}
}

func newGatedStore(inner pdf.Store, release <-chan struct{}) *gatedStore {
	return &gatedStore{inner: inner, release: release}
}

func (g *gatedStore) Ensure(ctx context.Context, key pdf.Key) (pdf.Locator, error) {
	select {
	case <-g.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return g.inner.Ensure(ctx, key)
}

func sampleRequests(n int) []paper.PDFDownloadRequest {
	out := make([]paper.PDFDownloadRequest, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.0000"+string(rune('1'+i)), "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.0000" + string(rune('1'+i)) + "v1.pdf",
		})
	}
	return out
}

func TestRegistry_SchedulePDFDownloads(t *testing.T) {
	t.Parallel()

	t.Run("returns a snapshot with one pending entry per request and a fresh job id", func(t *testing.T) {
		t.Parallel()
		reg, logger, _ := newRegistryForTest(t)
		reqs := sampleRequests(1)

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)

		if err != nil {
			t.Fatalf("SchedulePDFDownloads: unexpected error: %v", err)
		}
		if snap.JobID == "" {
			t.Fatalf("expected non-empty JobID")
		}
		if snap.Total != len(reqs) {
			t.Fatalf("Total = %d, want %d", snap.Total, len(reqs))
		}
		if snap.Completed {
			t.Fatalf("Completed = true, want false")
		}
		if snap.Succeeded != 0 || snap.Failed != 0 {
			t.Fatalf("counts = (succeeded=%d, failed=%d), want zero", snap.Succeeded, snap.Failed)
		}
		if !snap.CompletedAt.IsZero() {
			t.Fatalf("CompletedAt = %v, want zero", snap.CompletedAt)
		}
		if len(snap.Entries) != len(reqs) {
			t.Fatalf("Entries len = %d, want %d", len(snap.Entries), len(reqs))
		}
		for i, e := range snap.Entries {
			if e.Status != paper.DownloadStatusPending {
				t.Errorf("Entries[%d].Status = %q, want pending", i, e.Status)
			}
			if e.PaperID != reqs[i].PaperID {
				t.Errorf("Entries[%d].PaperID = %v, want %v", i, e.PaperID, reqs[i].PaperID)
			}
			if !e.CompletedAt.IsZero() {
				t.Errorf("Entries[%d].CompletedAt = %v, want zero", i, e.CompletedAt)
			}
		}

		records := logger.RecordsAt("Info")
		var found *mocks.LogRecord
		for i := range records {
			if records[i].Msg == "pdfdownload.job.scheduled" {
				found = &records[i]
				break
			}
		}
		if found == nil {
			t.Fatalf("expected an Info log with msg pdfdownload.job.scheduled, got %+v", logger.Records)
		}
		if got := found.Args["job_id"]; got != string(snap.JobID) {
			t.Errorf("log job_id = %v, want %v", got, snap.JobID)
		}
		if got := found.Args["total"]; got != len(reqs) {
			t.Errorf("log total = %v, want %d", got, len(reqs))
		}
		paperIDs, ok := found.Args["paper_ids"].([]string)
		if !ok {
			t.Fatalf("log paper_ids type = %T, want []string", found.Args["paper_ids"])
		}
		// Format mirrors design.md: "<source>:<sourceid>[v<version>]". Computed
		// inline so the test does not depend on a paper.ID method that lives
		// outside this task's boundary.
		want := []string{formatPaperID(reqs[0].PaperID)}
		if !reflect.DeepEqual(paperIDs, want) {
			t.Errorf("log paper_ids = %v, want %v", paperIDs, want)
		}
	})

	t.Run("yields a unique job id per call when scheduling many batches", func(t *testing.T) {
		t.Parallel()
		reg, logger, _ := newRegistryForTest(t)

		seen := map[paper.DownloadJobID]struct{}{}
		const calls = 5
		for i := 0; i < calls; i++ {
			reqs := sampleRequests(2)

			snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)

			if err != nil {
				t.Fatalf("call %d: unexpected error: %v", i, err)
			}
			if _, dup := seen[snap.JobID]; dup {
				t.Fatalf("duplicate JobID across calls: %q", snap.JobID)
			}
			seen[snap.JobID] = struct{}{}
			if snap.Total != len(reqs) {
				t.Errorf("call %d: Total = %d, want %d", i, snap.Total, len(reqs))
			}
			if len(snap.Entries) != len(reqs) {
				t.Errorf("call %d: Entries len = %d, want %d", i, len(snap.Entries), len(reqs))
			}
		}

		scheduledLogs := 0
		for _, r := range logger.RecordsAt("Info") {
			if r.Msg == "pdfdownload.job.scheduled" {
				scheduledLogs++
			}
		}
		if scheduledLogs != calls {
			t.Errorf("scheduled-log count = %d, want %d", scheduledLogs, calls)
		}
	})

	t.Run("short-circuits empty requests with a zero snapshot and no log", func(t *testing.T) {
		t.Parallel()
		reg, logger, _ := newRegistryForTest(t)

		snap, err := reg.SchedulePDFDownloads(context.Background(), nil)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		zero := paper.DownloadJobSnapshot{}
		if !reflect.DeepEqual(snap, zero) {
			t.Errorf("snapshot = %+v, want zero value", snap)
		}
		for _, r := range logger.Records {
			if r.Msg == "pdfdownload.job.scheduled" {
				t.Errorf("expected no scheduled log for empty requests, got %+v", r)
			}
		}
	})

	t.Run("ignores caller ctx cancellation when registering state", func(t *testing.T) {
		t.Parallel()
		reg, _, _ := newRegistryForTest(t)
		reqs := sampleRequests(2)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		snap, err := reg.SchedulePDFDownloads(ctx, reqs)

		if err != nil {
			t.Fatalf("Schedule must not consult ctx for state mutation; got error %v", err)
		}
		if snap.JobID == "" {
			t.Fatalf("expected a job to be registered even with cancelled ctx")
		}
		if snap.Total != len(reqs) {
			t.Errorf("Total = %d, want %d", snap.Total, len(reqs))
		}
	})
}

func TestRegistry_Reader_UnknownID(t *testing.T) {
	t.Parallel()

	t.Run("snapshot returns ErrDownloadJobUnknown for an id that was never scheduled", func(t *testing.T) {
		t.Parallel()
		reg, _, _ := newRegistryForTest(t)

		_, err := reg.SnapshotPDFDownloadJob(context.Background(), paper.DownloadJobID("does-not-exist"))

		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("err = %v, want ErrDownloadJobUnknown", err)
		}
	})

	t.Run("subscribe returns ErrDownloadJobUnknown for an id that was never scheduled", func(t *testing.T) {
		t.Parallel()
		reg, _, _ := newRegistryForTest(t)

		backlog, live, err := reg.SubscribePDFDownloadJob(context.Background(), paper.DownloadJobID("does-not-exist"))

		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("err = %v, want ErrDownloadJobUnknown", err)
		}
		if backlog != nil {
			t.Errorf("backlog = %v, want nil", backlog)
		}
		if live != nil {
			t.Errorf("live = %v, want nil", live)
		}
	})
}

func TestRegistry_SnapshotPDFDownloadJob(t *testing.T) {
	t.Parallel()

	t.Run("final snapshot for a completed job matches the Progress and Summary events delivered on the stream", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		const n = 4
		reqs := make([]paper.PDFDownloadRequest, 0, n)
		for i := 0; i < n; i++ {
			req := paper.PDFDownloadRequest{
				PaperID: paper.NewID("arxiv", "2404.0snap0"+string(rune('1'+i)), "v1"),
				PDFURL:  "https://arxiv.org/pdf/snap-" + string(rune('1'+i)) + ".pdf",
			}
			inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("body-" + string(rune('1'+i)))}
			reqs = append(reqs, req)
		}

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// Subscribe BEFORE releasing the worker so live carries every event.
		backlog, live, err := reg.SubscribePDFDownloadJob(context.Background(), snap.JobID)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		if len(backlog) != 0 {
			t.Fatalf("pre-emit backlog len = %d, want 0", len(backlog))
		}
		close(release)

		events, closed := drainEvents(live, 2*time.Second)
		if !closed {
			t.Fatalf("live did not close; got %d events", len(events))
		}
		if len(events) != n+1 {
			t.Fatalf("events len = %d, want %d", len(events), n+1)
		}

		final, err := reg.SnapshotPDFDownloadJob(context.Background(), snap.JobID)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}

		summary := events[n].Summary
		if summary == nil {
			t.Fatalf("expected terminal Summary event")
		}
		if final.JobID != summary.JobID {
			t.Errorf("snapshot JobID = %q, want %q", final.JobID, summary.JobID)
		}
		if final.Total != summary.Total {
			t.Errorf("snapshot Total = %d, want %d", final.Total, summary.Total)
		}
		if final.Succeeded != summary.Succeeded {
			t.Errorf("snapshot Succeeded = %d, want %d", final.Succeeded, summary.Succeeded)
		}
		if final.Failed != summary.Failed {
			t.Errorf("snapshot Failed = %d, want %d", final.Failed, summary.Failed)
		}
		if !final.Completed {
			t.Errorf("snapshot Completed = false, want true")
		}
		if final.CompletedAt != summary.CompletedAt {
			t.Errorf("snapshot CompletedAt = %v, want %v", final.CompletedAt, summary.CompletedAt)
		}

		// Per-entry consistency: the i-th Progress event's result must equal
		// the i-th snapshot entry (R5.4 — stream and status agree).
		if len(final.Entries) != n {
			t.Fatalf("snapshot Entries len = %d, want %d", len(final.Entries), n)
		}
		for i := 0; i < n; i++ {
			progress := events[i].Progress
			if progress == nil {
				t.Fatalf("events[%d].Progress = nil", i)
			}
			if !reflect.DeepEqual(final.Entries[i], *progress) {
				t.Errorf("snapshot Entries[%d] = %+v, progress event = %+v", i, final.Entries[i], *progress)
			}
		}
	})

	t.Run("snapshot returned to a caller cannot mutate registry-internal entries", func(t *testing.T) {
		t.Parallel()

		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.copy01", "v1"),
			PDFURL:  "https://arxiv.org/pdf/copy.pdf",
		}
		store.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("xx")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, schedSnap.JobID, 2*time.Second)

		snap, err := reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		// Mutate the returned slice; the next snapshot must be unaffected.
		snap.Entries[0].Status = paper.DownloadStatusFailed

		snap2, err := reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)
		if err != nil {
			t.Fatalf("Snapshot 2: %v", err)
		}
		if snap2.Entries[0].Status != paper.DownloadStatusSuccess {
			t.Errorf("registry-internal entry was mutated through returned snapshot; status = %q", snap2.Entries[0].Status)
		}
	})

	t.Run("snapshot for an in-progress job reflects pending entries with completed=false", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		t.Cleanup(func() {
			// Release the worker so shutdown can drain even if the test fails early.
			select {
			case <-release:
			default:
				close(release)
			}
		})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.prog01", "v1"),
			PDFURL:  "https://arxiv.org/pdf/prog.pdf",
		}
		inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("p")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// Worker is gated, so the entry is still Pending. R5.2.
		snap, err := reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)
		if err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if snap.Completed {
			t.Errorf("Completed = true, want false for gated job")
		}
		if snap.Succeeded != 0 || snap.Failed != 0 {
			t.Errorf("counts = (succ=%d, fail=%d), want zero for gated job", snap.Succeeded, snap.Failed)
		}
		if len(snap.Entries) != 1 || snap.Entries[0].Status != paper.DownloadStatusPending {
			t.Errorf("entries = %+v, want one pending", snap.Entries)
		}
	})
}

func TestRegistry_SubscribePDFDownloadJob(t *testing.T) {
	t.Parallel()

	t.Run("subscriber attached after some entries have been emitted receives a backlog plus the remaining events on live", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		const n = 4
		reqs := make([]paper.PDFDownloadRequest, 0, n)
		for i := 0; i < n; i++ {
			req := paper.PDFDownloadRequest{
				PaperID: paper.NewID("arxiv", "2404.0sub0"+string(rune('1'+i)), "v1"),
				PDFURL:  "https://arxiv.org/pdf/sub-" + string(rune('1'+i)) + ".pdf",
			}
			inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("b")}
			reqs = append(reqs, req)
		}

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// Subscribe BEFORE the worker is released — both backlog and live
		// must combine to exactly n+1 events with no duplicates.
		close(release)
		backlog, live, err := reg.SubscribePDFDownloadJob(context.Background(), snap.JobID)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}

		got := append([]paper.DownloadEvent(nil), backlog...)
		liveEvents, closed := drainEvents(live, 2*time.Second)
		if !closed {
			t.Fatalf("live channel did not close; got %d live events on top of %d backlog", len(liveEvents), len(backlog))
		}
		got = append(got, liveEvents...)

		if len(got) != n+1 {
			t.Fatalf("backlog+live total = %d, want %d", len(got), n+1)
		}
		for i := 0; i < n; i++ {
			if got[i].Progress == nil {
				t.Errorf("event[%d].Progress = nil, want non-nil", i)
			}
			if got[i].Summary != nil {
				t.Errorf("event[%d].Summary = non-nil, want nil", i)
			}
		}
		if got[n].Summary == nil {
			t.Errorf("event[n].Summary = nil, want non-nil (terminal event)")
		}
	})

	t.Run("subscriber attached after the job completed receives the full backlog and a pre-closed live channel", func(t *testing.T) {
		t.Parallel()

		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.post01", "v1"),
			PDFURL:  "https://arxiv.org/pdf/post.pdf",
		}
		store.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("done")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, schedSnap.JobID, 2*time.Second)

		backlog, live, err := reg.SubscribePDFDownloadJob(context.Background(), schedSnap.JobID)
		if err != nil {
			t.Fatalf("Subscribe post-completion: %v", err)
		}
		if len(backlog) != 2 {
			t.Fatalf("backlog len = %d, want 2 (1 progress + 1 summary)", len(backlog))
		}
		if backlog[0].Progress == nil {
			t.Errorf("backlog[0].Progress = nil, want non-nil")
		}
		if backlog[1].Summary == nil {
			t.Errorf("backlog[1].Summary = nil, want non-nil")
		}
		// Live must be already closed: no more events ever fire for a completed job.
		select {
		case _, ok := <-live:
			if ok {
				t.Errorf("live yielded a value after completion; want closed channel")
			}
		case <-time.After(500 * time.Millisecond):
			t.Errorf("live channel was not closed after completion")
		}
	})

	t.Run("N concurrent subscribers all see every event exactly once under -race", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		// Large per-subscriber buffer so contention does not trigger the
		// slow-consumer drop path — the focus here is the atomic handoff
		// between backlog and live, not the drop policy.
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 256,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		const n = 12
		reqs := make([]paper.PDFDownloadRequest, 0, n)
		for i := 0; i < n; i++ {
			req := paper.PDFDownloadRequest{
				PaperID: paper.NewID("arxiv", fmt.Sprintf("2404.0cnt%02d", i), "v1"),
				PDFURL:  fmt.Sprintf("https://arxiv.org/pdf/cnt-%02d.pdf", i),
			}
			inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("c")}
			reqs = append(reqs, req)
		}

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		const subscribers = 16
		// Use a barrier so all subscriber goroutines try to subscribe while
		// the worker is mid-flight. Releasing the worker concurrently with
		// the subscribe attempts is what stresses the atomic backlog handoff.
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make(chan int, subscribers)
		errs := make(chan error, subscribers)

		for s := 0; s < subscribers; s++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				backlog, live, subErr := reg.SubscribePDFDownloadJob(context.Background(), snap.JobID)
				if subErr != nil {
					errs <- fmt.Errorf("subscribe: %w", subErr)
					return
				}
				count := len(backlog)
				// Drain live until close; count every event so duplicates
				// or losses turn into a count mismatch.
				timer := time.NewTimer(5 * time.Second)
				defer timer.Stop()
				for {
					select {
					case _, ok := <-live:
						if !ok {
							results <- count
							return
						}
						count++
					case <-timer.C:
						errs <- fmt.Errorf("subscriber timed out after %d events", count)
						return
					}
				}
			}()
		}

		// Fire all subscribers and the worker as close to simultaneously as
		// possible. The race detector and the per-subscriber count assertion
		// together catch lost or duplicated events.
		close(start)
		close(release)

		wg.Wait()
		close(results)
		close(errs)

		for e := range errs {
			t.Fatalf("subscriber goroutine failed: %v", e)
		}

		seen := 0
		for c := range results {
			seen++
			if c != n+1 {
				t.Errorf("subscriber saw %d events, want %d (n progress + 1 summary)", c, n+1)
			}
		}
		if seen != subscribers {
			t.Errorf("collected %d results, want %d", seen, subscribers)
		}
	})
}

// waitForJobCompletion polls the registry-internal completion flag for jobID
// up to deadline. It surfaces a hard failure if the worker does not finish in
// time so the suite cannot hang under regression.
func waitForJobCompletion(t *testing.T, reg *pdfdownload.Registry, jobID paper.DownloadJobID, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if reg.JobCompletedForTest(jobID) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("worker for job %q did not finish within %s", jobID, deadline)
}

func TestRegistry_Worker(t *testing.T) {
	t.Parallel()

	t.Run("records a successful download with status success and a non-zero byte count", func(t *testing.T) {
		t.Parallel()
		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.00001", "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.00001v1.pdf",
		}
		key := pdf.Key{
			SourceType: req.PaperID.Source,
			SourceID:   req.PaperID.PDFArtifactKey(),
			URL:        req.PDFURL,
		}
		body := []byte("hello-pdf-bytes")
		store.Responses[key] = mocks.PDFStoreResponse{Body: body}

		snap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})

		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, snap.JobID, 2*time.Second)

		entries := reg.JobEntriesForTest(snap.JobID)
		if len(entries) != 1 {
			t.Fatalf("entries len = %d, want 1", len(entries))
		}
		got := entries[0]
		if got.Status != paper.DownloadStatusSuccess {
			t.Errorf("status = %q, want %q", got.Status, paper.DownloadStatusSuccess)
		}
		if got.Bytes != len(body) {
			t.Errorf("bytes = %d, want %d", got.Bytes, len(body))
		}
		if got.PaperID != req.PaperID {
			t.Errorf("paper id = %v, want %v", got.PaperID, req.PaperID)
		}
		if got.Category != "" {
			t.Errorf("category = %q, want empty on success", got.Category)
		}

		var found *mocks.LogRecord
		for _, r := range logger.RecordsAt("Info") {
			if r.Msg == "pdfdownload.entry.completed" {
				found = &r
				break
			}
		}
		if found == nil {
			t.Fatalf("expected Info log pdfdownload.entry.completed; records=%+v", logger.Records)
		}
		if found.Args["status"] != string(paper.DownloadStatusSuccess) {
			t.Errorf("log status = %v, want %q", found.Args["status"], paper.DownloadStatusSuccess)
		}
		if got := found.Args["job_id"]; got != string(snap.JobID) {
			t.Errorf("log job_id = %v, want %v", got, snap.JobID)
		}
		if got := found.Args["bytes"]; got != len(body) {
			t.Errorf("log bytes = %v, want %d", got, len(body))
		}
	})

	t.Run("classifies an ErrFetch outcome as failed/fetch and keeps processing the next entry", func(t *testing.T) {
		t.Parallel()
		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		badReq := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.00001", "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.00001v1.pdf",
		}
		goodReq := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.00002", "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.00002v1.pdf",
		}
		badKey := pdf.Key{SourceType: badReq.PaperID.Source, SourceID: badReq.PaperID.PDFArtifactKey(), URL: badReq.PDFURL}
		goodKey := pdf.Key{SourceType: goodReq.PaperID.Source, SourceID: goodReq.PaperID.PDFArtifactKey(), URL: goodReq.PDFURL}
		store.Responses[badKey] = mocks.PDFStoreResponse{Err: fmt.Errorf("upstream 500: %w", pdf.ErrFetch)}
		goodBody := []byte("ok-bytes-456")
		store.Responses[goodKey] = mocks.PDFStoreResponse{Body: goodBody}

		snap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{badReq, goodReq})

		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, snap.JobID, 2*time.Second)

		entries := reg.JobEntriesForTest(snap.JobID)
		if len(entries) != 2 {
			t.Fatalf("entries len = %d, want 2", len(entries))
		}
		if entries[0].Status != paper.DownloadStatusFailed {
			t.Errorf("entries[0].Status = %q, want %q", entries[0].Status, paper.DownloadStatusFailed)
		}
		if entries[0].Category != pdf.CategoryFetch {
			t.Errorf("entries[0].Category = %q, want %q", entries[0].Category, pdf.CategoryFetch)
		}
		if entries[0].Description == "" {
			t.Errorf("entries[0].Description must not be empty on failure")
		}
		if entries[1].Status != paper.DownloadStatusSuccess {
			t.Errorf("entries[1].Status = %q, want %q (loop must continue past failure)", entries[1].Status, paper.DownloadStatusSuccess)
		}
		if entries[1].Bytes != len(goodBody) {
			t.Errorf("entries[1].Bytes = %d, want %d", entries[1].Bytes, len(goodBody))
		}

		var warn *mocks.LogRecord
		for _, r := range logger.RecordsAt("Warn") {
			if r.Msg == "pdfdownload.entry.completed" {
				warn = &r
				break
			}
		}
		if warn == nil {
			t.Fatalf("expected Warn log pdfdownload.entry.completed; records=%+v", logger.Records)
		}
		if warn.Args["category"] != pdf.CategoryFetch {
			t.Errorf("log category = %v, want %q", warn.Args["category"], pdf.CategoryFetch)
		}
	})

	t.Run("runs to completion even when the caller ctx is cancelled before the worker starts", func(t *testing.T) {
		t.Parallel()
		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.00003", "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.00003v1.pdf",
		}
		key := pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}
		body := []byte("payload-xyz")
		store.Responses[key] = mocks.PDFStoreResponse{Body: body}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		snap, err := reg.SchedulePDFDownloads(ctx, []paper.PDFDownloadRequest{req})

		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, snap.JobID, 2*time.Second)

		entries := reg.JobEntriesForTest(snap.JobID)
		if len(entries) != 1 {
			t.Fatalf("entries len = %d, want 1", len(entries))
		}
		if entries[0].Status != paper.DownloadStatusSuccess {
			t.Errorf("status = %q, want %q (worker must ignore caller ctx)", entries[0].Status, paper.DownloadStatusSuccess)
		}
		if entries[0].Bytes != len(body) {
			t.Errorf("bytes = %d, want %d", entries[0].Bytes, len(body))
		}
	})
}

// drainEvents reads events from ch until it is closed or deadline expires.
// Returns the events received in order and ok=true if the channel closed
// before deadline; ok=false means the deadline tripped (test should fail).
func drainEvents(ch <-chan paper.DownloadEvent, deadline time.Duration) ([]paper.DownloadEvent, bool) {
	var out []paper.DownloadEvent
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out, true
			}
			out = append(out, ev)
		case <-timer.C:
			return out, false
		}
	}
}

func TestRegistry_FanOut(t *testing.T) {
	t.Parallel()

	t.Run("draining subscriber receives every progress event followed by summary and channel close", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		clock := mocks.NewMovableClock(startedAt)
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		const n = 3
		reqs := make([]paper.PDFDownloadRequest, 0, n)
		for i := 0; i < n; i++ {
			req := paper.PDFDownloadRequest{
				PaperID: paper.NewID("arxiv", "2404.0fan0"+string(rune('1'+i)), "v1"),
				PDFURL:  "https://arxiv.org/pdf/fanout-" + string(rune('1'+i)) + ".pdf",
			}
			inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("body-" + string(rune('1'+i)))}
			reqs = append(reqs, req)
		}

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		ch, err := reg.AttachTestSubscriberForJob(snap.JobID, 16)
		if err != nil {
			t.Fatalf("AttachTestSubscriberForJob: %v", err)
		}
		close(release)

		events, closed := drainEvents(ch, 2*time.Second)
		if !closed {
			t.Fatalf("subscriber channel did not close within deadline; got %d events", len(events))
		}
		if len(events) != n+1 {
			t.Fatalf("event count = %d, want %d (n progress + 1 summary)", len(events), n+1)
		}
		for i := 0; i < n; i++ {
			if events[i].Progress == nil {
				t.Errorf("events[%d].Progress = nil, want non-nil", i)
			}
			if events[i].Summary != nil {
				t.Errorf("events[%d].Summary = non-nil, want nil", i)
			}
		}
		last := events[n]
		if last.Summary == nil {
			t.Fatalf("last event.Summary = nil, want non-nil")
		}
		if last.Progress != nil {
			t.Errorf("last event.Progress = non-nil, want nil on terminal Summary")
		}
		if last.Summary.JobID != snap.JobID {
			t.Errorf("summary JobID = %q, want %q", last.Summary.JobID, snap.JobID)
		}
		if last.Summary.Total != n {
			t.Errorf("summary Total = %d, want %d", last.Summary.Total, n)
		}
		if last.Summary.Succeeded != n {
			t.Errorf("summary Succeeded = %d, want %d", last.Summary.Succeeded, n)
		}
		if last.Summary.Failed != 0 {
			t.Errorf("summary Failed = %d, want 0", last.Summary.Failed)
		}
		if !last.Summary.Completed {
			t.Errorf("summary Completed = false, want true")
		}

		// Events log is bounded by total+1 and must match what the subscriber saw.
		log := reg.JobEventsForTest(snap.JobID)
		if len(log) != n+1 {
			t.Errorf("events log len = %d, want %d", len(log), n+1)
		}

		var completedLog *mocks.LogRecord
		for _, r := range logger.RecordsAt("Info") {
			if r.Msg == "pdfdownload.job.completed" {
				rec := r
				completedLog = &rec
				break
			}
		}
		if completedLog == nil {
			t.Fatalf("expected Info log pdfdownload.job.completed; records=%+v", logger.Records)
		}
		if completedLog.Args["job_id"] != string(snap.JobID) {
			t.Errorf("completed log job_id = %v, want %v", completedLog.Args["job_id"], snap.JobID)
		}
		if completedLog.Args["total"] != n {
			t.Errorf("completed log total = %v, want %d", completedLog.Args["total"], n)
		}
		if completedLog.Args["succeeded"] != n {
			t.Errorf("completed log succeeded = %v, want %d", completedLog.Args["succeeded"], n)
		}
		if completedLog.Args["failed"] != 0 {
			t.Errorf("completed log failed = %v, want 0", completedLog.Args["failed"])
		}
		if _, ok := completedLog.Args["duration_ms"]; !ok {
			t.Errorf("completed log missing duration_ms; args=%+v", completedLog.Args)
		}
	})

	t.Run("slow subscriber whose channel overflows is dropped without stalling the worker", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		// SubscriberBuffer here covers the public Subscribe contract that
		// task 2.4 will wire; the test-only AttachTestSubscriberForJob
		// takes an explicit small bufSize to drive the overflow path.
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		const n = 5
		reqs := make([]paper.PDFDownloadRequest, 0, n)
		for i := 0; i < n; i++ {
			req := paper.PDFDownloadRequest{
				PaperID: paper.NewID("arxiv", "2404.0slow0"+string(rune('1'+i)), "v1"),
				PDFURL:  "https://arxiv.org/pdf/slow-" + string(rune('1'+i)) + ".pdf",
			}
			inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("payload")}
			reqs = append(reqs, req)
		}

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// Tiny buffer (1) guarantees the second progress event hits the
		// non-blocking-send default branch, dropping the subscriber.
		slow, err := reg.AttachTestSubscriberForJob(snap.JobID, 1)
		if err != nil {
			t.Fatalf("AttachTestSubscriberForJob: %v", err)
		}
		close(release)

		// The slow subscriber never reads. The worker must still complete.
		waitForJobCompletion(t, reg, snap.JobID, 2*time.Second)

		// After being dropped, the channel is closed. The first send filled the
		// buffer, so we expect to read exactly one buffered event followed by close.
		read, closed := drainEvents(slow, 1*time.Second)
		if !closed {
			t.Fatalf("slow subscriber channel did not close; read %d events", len(read))
		}
		if len(read) > 1 {
			t.Errorf("slow subscriber received %d events, want at most 1 before drop", len(read))
		}

		var dropLog *mocks.LogRecord
		for _, r := range logger.RecordsAt("Warn") {
			if r.Msg == "pdfdownload.subscriber.dropped" {
				rec := r
				dropLog = &rec
				break
			}
		}
		if dropLog == nil {
			t.Fatalf("expected Warn log pdfdownload.subscriber.dropped; records=%+v", logger.Records)
		}
		if dropLog.Args["job_id"] != string(snap.JobID) {
			t.Errorf("drop log job_id = %v, want %v", dropLog.Args["job_id"], snap.JobID)
		}
		if dropLog.Args["reason"] != "slow_consumer" {
			t.Errorf("drop log reason = %v, want %q", dropLog.Args["reason"], "slow_consumer")
		}
	})

	t.Run("summary still emits when every entry fails", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		const n = 3
		reqs := make([]paper.PDFDownloadRequest, 0, n)
		for i := 0; i < n; i++ {
			req := paper.PDFDownloadRequest{
				PaperID: paper.NewID("arxiv", "2404.0fail0"+string(rune('1'+i)), "v1"),
				PDFURL:  "https://arxiv.org/pdf/fail-" + string(rune('1'+i)) + ".pdf",
			}
			inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Err: fmt.Errorf("upstream 503: %w", pdf.ErrFetch)}
			reqs = append(reqs, req)
		}

		snap, err := reg.SchedulePDFDownloads(context.Background(), reqs)
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		ch, err := reg.AttachTestSubscriberForJob(snap.JobID, 16)
		if err != nil {
			t.Fatalf("AttachTestSubscriberForJob: %v", err)
		}
		close(release)

		events, closed := drainEvents(ch, 2*time.Second)
		if !closed {
			t.Fatalf("subscriber channel did not close; got %d events", len(events))
		}
		if len(events) != n+1 {
			t.Fatalf("event count = %d, want %d", len(events), n+1)
		}
		summary := events[n].Summary
		if summary == nil {
			t.Fatalf("expected terminal Summary event, got Progress")
		}
		if summary.Total != n {
			t.Errorf("summary Total = %d, want %d", summary.Total, n)
		}
		if summary.Succeeded != 0 {
			t.Errorf("summary Succeeded = %d, want 0", summary.Succeeded)
		}
		if summary.Failed != n {
			t.Errorf("summary Failed = %d, want %d", summary.Failed, n)
		}
		if !summary.Completed {
			t.Errorf("summary Completed = false, want true")
		}
	})
}

func TestRegistry_Sweep(t *testing.T) {
	t.Parallel()

	t.Run("retains a completed job whose age is below the retention window", func(t *testing.T) {
		t.Parallel()

		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		clock := mocks.NewMovableClock(start)
		retention := 5 * time.Minute
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        retention,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.retain1", "v1"),
			PDFURL:  "https://arxiv.org/pdf/retain1.pdf",
		}
		store.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("body")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, schedSnap.JobID, 2*time.Second)

		clock.Set(start.Add(retention - time.Second))

		snap, err := reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)

		if err != nil {
			t.Fatalf("Snapshot: unexpected error %v", err)
		}
		if snap.JobID != schedSnap.JobID {
			t.Errorf("snapshot JobID = %q, want %q", snap.JobID, schedSnap.JobID)
		}
		if !snap.Completed {
			t.Errorf("snapshot Completed = false, want true")
		}
		for _, r := range logger.RecordsAt("Info") {
			if r.Msg == "pdfdownload.job.evicted" {
				t.Errorf("unexpected eviction log while age < retention: %+v", r)
			}
		}
	})

	t.Run("never evicts an in-progress job even when age exceeds the retention window", func(t *testing.T) {
		t.Parallel()

		inner := mocks.NewPDFStore(t.TempDir())
		release := make(chan struct{})
		t.Cleanup(func() {
			// Always release so the worker can finish before shutdown drains.
			select {
			case <-release:
			default:
				close(release)
			}
		})
		store := newGatedStore(inner, release)
		logger := &mocks.RecordingLogger{}
		start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		clock := mocks.NewMovableClock(start)
		retention := 5 * time.Minute
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        retention,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.gated01", "v1"),
			PDFURL:  "https://arxiv.org/pdf/gated.pdf",
		}
		inner.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("g")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}

		// Worker is blocked on the gated store; advance the clock past 2x
		// the retention window and force a sweep directly.
		clock.Set(start.Add(2 * retention))
		reg.Sweep(context.Background(), clock.Now())

		snap, err := reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)
		if err != nil {
			t.Fatalf("Snapshot after sweep on in-progress job: %v", err)
		}
		if snap.JobID != schedSnap.JobID {
			t.Errorf("snapshot JobID = %q, want %q", snap.JobID, schedSnap.JobID)
		}
		if snap.Completed {
			t.Errorf("Completed = true, want false (worker is gated)")
		}
		for _, r := range logger.RecordsAt("Info") {
			if r.Msg == "pdfdownload.job.evicted" {
				t.Errorf("unexpected eviction log for in-progress job: %+v", r)
			}
		}

		// Release the worker so the test does not leave a goroutine hanging.
		close(release)
		waitForJobCompletion(t, reg, schedSnap.JobID, 2*time.Second)
	})

	t.Run("evicts a completed job older than the retention window on the next snapshot read", func(t *testing.T) {
		t.Parallel()

		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		clock := mocks.NewMovableClock(start)
		retention := 5 * time.Minute
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        retention,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.evict01", "v1"),
			PDFURL:  "https://arxiv.org/pdf/evict.pdf",
		}
		store.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("e")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, schedSnap.JobID, 2*time.Second)

		advance := retention + time.Second
		clock.Set(start.Add(advance))

		_, err = reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)
		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("Snapshot after eviction: err = %v, want ErrDownloadJobUnknown", err)
		}

		backlog, live, err := reg.SubscribePDFDownloadJob(context.Background(), schedSnap.JobID)
		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("Subscribe after eviction: err = %v, want ErrDownloadJobUnknown", err)
		}
		if backlog != nil {
			t.Errorf("backlog = %v, want nil after eviction", backlog)
		}
		if live != nil {
			t.Errorf("live = %v, want nil after eviction", live)
		}

		var evictLog *mocks.LogRecord
		for _, r := range logger.RecordsAt("Info") {
			if r.Msg == "pdfdownload.job.evicted" {
				rec := r
				evictLog = &rec
				break
			}
		}
		if evictLog == nil {
			t.Fatalf("expected pdfdownload.job.evicted log; records=%+v", logger.Records)
		}
		if got := evictLog.Args["job_id"]; got != string(schedSnap.JobID) {
			t.Errorf("evict log job_id = %v, want %v", got, schedSnap.JobID)
		}
		ageMs, ok := evictLog.Args["age_ms"].(int64)
		if !ok {
			t.Fatalf("evict log age_ms type = %T, want int64", evictLog.Args["age_ms"])
		}
		if ageMs <= 0 {
			t.Errorf("evict log age_ms = %d, want > 0", ageMs)
		}
	})

	t.Run("explicit Sweep call evicts an aged completed job before any read path runs", func(t *testing.T) {
		t.Parallel()

		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		clock := mocks.NewMovableClock(start)
		retention := 5 * time.Minute
		reg, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        retention,
			SubscriberBuffer: 32,
		})
		t.Cleanup(func() { _ = shutdown(context.Background()) })

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("arxiv", "2404.swdir01", "v1"),
			PDFURL:  "https://arxiv.org/pdf/swdir.pdf",
		}
		store.Responses[pdf.Key{SourceType: req.PaperID.Source, SourceID: req.PaperID.PDFArtifactKey(), URL: req.PDFURL}] = mocks.PDFStoreResponse{Body: []byte("s")}

		schedSnap, err := reg.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{req})
		if err != nil {
			t.Fatalf("Schedule: %v", err)
		}
		waitForJobCompletion(t, reg, schedSnap.JobID, 2*time.Second)

		clock.Set(start.Add(retention + 2*time.Second))
		reg.Sweep(context.Background(), clock.Now())

		_, err = reg.SnapshotPDFDownloadJob(context.Background(), schedSnap.JobID)

		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("Snapshot after direct Sweep: err = %v, want ErrDownloadJobUnknown", err)
		}
	})
}

func TestNewRegistry_ShutdownReturnsNil(t *testing.T) {
	t.Parallel()

	t.Run("calling shutdown after construction reports no error and is safe to repeat", func(t *testing.T) {
		t.Parallel()
		store := mocks.NewPDFStore(t.TempDir())
		logger := &mocks.RecordingLogger{}
		clock := mocks.NewMovableClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		_, shutdown := pdfdownload.NewRegistry(store, logger, clock, pdfdownload.Options{
			Retention:        5 * time.Minute,
			SubscriberBuffer: 32,
		})

		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown 1: unexpected error: %v", err)
		}
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown 2: unexpected error: %v", err)
		}
	})
}
