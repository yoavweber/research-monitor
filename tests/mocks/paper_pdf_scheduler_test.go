package mocks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

func TestPaperPDFScheduler(t *testing.T) {
	t.Parallel()

	t.Run("empty requests returns a zero snapshot and no recorded job", func(t *testing.T) {
		t.Parallel()
		s := mocks.NewPaperPDFScheduler()

		snap, err := s.SchedulePDFDownloads(context.Background(), nil)

		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if snap.JobID != "" {
			t.Errorf("expected zero snapshot, got JobID=%q", snap.JobID)
		}
		if s.CallCount() != 1 {
			t.Errorf("CallCount = %d, want 1", s.CallCount())
		}
	})

	t.Run("non-empty requests synthesizes a pending snapshot and records the call", func(t *testing.T) {
		t.Parallel()
		s := mocks.NewPaperPDFScheduler()

		reqs := []paper.PDFDownloadRequest{
			{PaperID: paper.NewID(paper.SourceArxiv, "1", "v1"), PDFURL: "http://x/1"},
			{PaperID: paper.NewID(paper.SourceArxiv, "2", ""), PDFURL: "http://x/2"},
		}

		snap, err := s.SchedulePDFDownloads(context.Background(), reqs)

		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if snap.JobID == "" {
			t.Error("expected non-empty JobID")
		}
		if snap.Total != 2 || len(snap.Entries) != 2 {
			t.Errorf("snapshot = %+v, want Total=2 with 2 Entries", snap)
		}
		if snap.Entries[0].Status != paper.DownloadStatusPending {
			t.Errorf("Entries[0].Status = %q, want pending", snap.Entries[0].Status)
		}
		got := s.LastCall()
		if len(got) != 2 || got[0] != reqs[0] || got[1] != reqs[1] {
			t.Errorf("LastCall = %+v, want %+v", got, reqs)
		}
	})

	t.Run("configured Err is returned without using the request slice", func(t *testing.T) {
		t.Parallel()
		s := mocks.NewPaperPDFScheduler()
		boom := errors.New("registry shutting down")
		s.Err = boom

		_, err := s.SchedulePDFDownloads(context.Background(), []paper.PDFDownloadRequest{
			{PaperID: paper.NewID(paper.SourceArxiv, "1", "v1"), PDFURL: "http://x/1"},
		})

		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
	})
}
