package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func TestPaperPDFDownloadReader(t *testing.T) {
	t.Parallel()

	t.Run("zero value models an unknown job for Snapshot", func(t *testing.T) {
		t.Parallel()
		r := mocks.NewPaperPDFDownloadReader()

		_, err := r.SnapshotPDFDownloadJob(context.Background(), "missing")

		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("err = %v, want ErrDownloadJobUnknown", err)
		}
		if r.SnapshotCallCount() != 1 {
			t.Fatalf("SnapshotCallCount = %d, want 1", r.SnapshotCallCount())
		}
		if r.LastSnapshotCall() != "missing" {
			t.Errorf("LastSnapshotCall = %q, want missing", r.LastSnapshotCall())
		}
	})

	t.Run("programmed snapshot is returned verbatim and the call is recorded", func(t *testing.T) {
		t.Parallel()
		r := mocks.NewPaperPDFDownloadReader()
		r.SnapshotErr = nil
		r.Snapshot = paper.DownloadJobSnapshot{
			JobID:     "job-1",
			Total:     2,
			Succeeded: 1,
			Failed:    1,
			Completed: true,
		}

		got, err := r.SnapshotPDFDownloadJob(context.Background(), "job-1")

		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if got.JobID != "job-1" || got.Total != 2 || got.Succeeded != 1 || got.Failed != 1 || !got.Completed {
			t.Errorf("snapshot = %+v, want JobID=job-1 totals (2,1,1) completed=true", got)
		}
	})

	t.Run("zero value models an unknown job for Subscribe", func(t *testing.T) {
		t.Parallel()
		r := mocks.NewPaperPDFDownloadReader()

		backlog, live, err := r.SubscribePDFDownloadJob(context.Background(), "missing")

		if !errors.Is(err, paper.ErrDownloadJobUnknown) {
			t.Fatalf("err = %v, want ErrDownloadJobUnknown", err)
		}
		if backlog != nil || live != nil {
			t.Errorf("backlog=%v live=%v, want both nil", backlog, live)
		}
		if len(r.SubscribeCalls) != 1 || r.SubscribeCalls[0] != "missing" {
			t.Errorf("SubscribeCalls = %v, want [missing]", r.SubscribeCalls)
		}
	})
}
