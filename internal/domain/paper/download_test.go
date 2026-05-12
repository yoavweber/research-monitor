package paper_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

func TestNewPDFDownloadRequests(t *testing.T) {
	t.Parallel()

	t.Run("maps each entry into a request carrying its identity and pdf url", func(t *testing.T) {
		t.Parallel()

		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1", PDFURL: "https://arxiv.org/pdf/2404.12345v1"},
			{Source: paper.SourceArxiv, SourceID: "2405.67890", Version: "", PDFURL: "https://arxiv.org/pdf/2405.67890"},
		}

		got := paper.NewPDFDownloadRequests(entries)

		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if (got[0].PaperID != paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1"}) {
			t.Errorf("got[0].PaperID = %+v", got[0].PaperID)
		}
		if got[0].PDFURL != "https://arxiv.org/pdf/2404.12345v1" {
			t.Errorf("got[0].PDFURL = %q", got[0].PDFURL)
		}
		if (got[1].PaperID != paper.ID{Source: paper.SourceArxiv, SourceID: "2405.67890"}) {
			t.Errorf("got[1].PaperID = %+v", got[1].PaperID)
		}
	})

	t.Run("empty input yields an empty non-nil slice", func(t *testing.T) {
		t.Parallel()

		got := paper.NewPDFDownloadRequests(nil)

		if got == nil {
			t.Fatal("expected non-nil empty slice, got nil")
		}
		if len(got) != 0 {
			t.Fatalf("len = %d, want 0", len(got))
		}
	})
}
