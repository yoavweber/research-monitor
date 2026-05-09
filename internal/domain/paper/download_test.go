package paper_test

import (
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

func TestPDFDownloadRequestValidate(t *testing.T) {
	t.Parallel()

	t.Run("happy path arxiv-style request returns no error", func(t *testing.T) {
		t.Parallel()

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID(paper.SourceArxiv, "2404.12345", "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.12345v1",
		}

		err := req.Validate()

		if err != nil {
			t.Fatalf("expected nil error for valid request, got %v", err)
		}
	})

	t.Run("invalid paper id is surfaced as ErrInvalidID", func(t *testing.T) {
		t.Parallel()

		req := paper.PDFDownloadRequest{
			PaperID: paper.NewID("", "2404.12345", "v1"),
			PDFURL:  "https://arxiv.org/pdf/2404.12345v1",
		}

		err := req.Validate()

		if err == nil || !errors.Is(err, paper.ErrInvalidID) {
			t.Fatalf("expected ErrInvalidID, got %v", err)
		}
	})

	rejectionCases := []struct {
		name string
		req  paper.PDFDownloadRequest
	}{
		{
			name: "empty pdf url is rejected",
			req: paper.PDFDownloadRequest{
				PaperID: paper.NewID(paper.SourceArxiv, "2404.12345", "v1"),
				PDFURL:  "",
			},
		},
		{
			name: "whitespace-only pdf url is rejected",
			req: paper.PDFDownloadRequest{
				PaperID: paper.NewID(paper.SourceArxiv, "2404.12345", "v1"),
				PDFURL:  "  \t\n",
			},
		},
	}

	for _, tc := range rejectionCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.req.Validate()

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, paper.ErrInvalidPDFDownloadRequest) {
				t.Fatalf("expected error to wrap ErrInvalidPDFDownloadRequest, got %v", err)
			}
		})
	}
}

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
		if got[0].PaperID != paper.NewID(paper.SourceArxiv, "2404.12345", "v1") {
			t.Errorf("got[0].PaperID = %+v", got[0].PaperID)
		}
		if got[0].PDFURL != "https://arxiv.org/pdf/2404.12345v1" {
			t.Errorf("got[0].PDFURL = %q", got[0].PDFURL)
		}
		if got[1].PaperID != paper.NewID(paper.SourceArxiv, "2405.67890", "") {
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
