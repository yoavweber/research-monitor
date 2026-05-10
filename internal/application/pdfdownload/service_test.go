package pdfdownload_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
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

func TestRegistry_StubReader(t *testing.T) {
	t.Parallel()

	t.Run("snapshot returns ErrDownloadJobUnknown for any id while reader is a stub", func(t *testing.T) {
		t.Parallel()
		reg, _, _ := newRegistryForTest(t)

		_, err := reg.SnapshotPDFDownloadJob(context.Background(), paper.DownloadJobID("does-not-exist"))

		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("err = %v, want ErrDownloadJobUnknown", err)
		}
	})

	t.Run("subscribe returns ErrDownloadJobUnknown for any id while reader is a stub", func(t *testing.T) {
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
