package paper_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
)

func TestToDownloadEntryResultDTO(t *testing.T) {
	t.Parallel()

	t.Run("pending entry omits completed_at from JSON", func(t *testing.T) {
		t.Parallel()

		dto := paperctrl.ToDownloadEntryResultDTO(paper.DownloadEntryResult{
			PaperID: paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1"},
			Status:  paper.DownloadStatusPending,
		})

		raw, err := json.Marshal(dto)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if strings.Contains(string(raw), "completed_at") {
			t.Errorf("pending entry should omit completed_at, got %s", raw)
		}
	})

	t.Run("completed entry serializes a non-zero completed_at", func(t *testing.T) {
		t.Parallel()

		ts := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

		dto := paperctrl.ToDownloadEntryResultDTO(paper.DownloadEntryResult{
			PaperID:     paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1"},
			Status:      paper.DownloadStatusSuccess,
			Bytes:       1234,
			CompletedAt: ts,
		})

		if dto.CompletedAt == nil || !dto.CompletedAt.Equal(ts) {
			t.Fatalf("CompletedAt = %v, want %v", dto.CompletedAt, ts)
		}
	})
}

func TestToDownloadJobSnapshotDTO(t *testing.T) {
	t.Parallel()

	t.Run("empty Entries marshals to a non-nil empty array", func(t *testing.T) {
		t.Parallel()

		dto := paperctrl.ToDownloadJobSnapshotDTO(paper.DownloadJobSnapshot{
			JobID: paper.DownloadJobID("abc"),
			Total: 0,
		})

		raw, err := json.Marshal(dto)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !strings.Contains(string(raw), `"entries":[]`) {
			t.Errorf("expected entries:[], got %s", raw)
		}
	})

	t.Run("populated snapshot maps job_id totals and per-entry shape", func(t *testing.T) {
		t.Parallel()

		ts := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
		snap := paper.DownloadJobSnapshot{
			JobID:       "job-1",
			Total:       2,
			Succeeded:   1,
			Failed:      1,
			Completed:   true,
			CompletedAt: ts,
			Entries: []paper.DownloadEntryResult{
				{PaperID: paper.ID{Source: paper.SourceArxiv, SourceID: "1", Version: "v1"}, Status: paper.DownloadStatusSuccess, Bytes: 42, CompletedAt: ts},
				{PaperID: paper.ID{Source: paper.SourceArxiv, SourceID: "2", Version: "v1"}, Status: paper.DownloadStatusFailed, Category: "fetch", Description: "boom", CompletedAt: ts},
			},
		}

		dto := paperctrl.ToDownloadJobSnapshotDTO(snap)

		if dto.JobID != "job-1" || dto.Total != 2 || dto.Succeeded != 1 || dto.Failed != 1 || !dto.Completed {
			t.Errorf("dto = %+v", dto)
		}
		if len(dto.Entries) != 2 {
			t.Fatalf("len(Entries) = %d, want 2", len(dto.Entries))
		}
		if dto.Entries[0].Status != "success" || dto.Entries[0].Bytes != 42 {
			t.Errorf("Entries[0] = %+v", dto.Entries[0])
		}
		if dto.Entries[1].Status != "failed" || dto.Entries[1].Category != "fetch" {
			t.Errorf("Entries[1] = %+v", dto.Entries[1])
		}
	})
}
