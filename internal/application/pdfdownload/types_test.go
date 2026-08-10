package pdfdownload_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

func TestPDFArtifactKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   paper.ID
		want string
	}{
		{
			name: "arxiv versioned id concatenates source id and version",
			id:   paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1"},
			want: "2404.12345v1",
		},
		{
			name: "arxiv id without version returns source id only",
			id:   paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345"},
			want: "2404.12345",
		},
		{
			name: "non-arxiv source uses the same concatenation rule",
			id:   paper.ID{Source: "biorxiv", SourceID: "10.1101/2023.01.01.522123", Version: "v2"},
			want: "10.1101/2023.01.01.522123v2",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := pdfdownload.PDFArtifactKey(tc.id)

			if got != tc.want {
				t.Fatalf("PDFArtifactKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequestsFromEntries(t *testing.T) {
	t.Parallel()

	t.Run("happy path produces one request per entry preserving order", func(t *testing.T) {
		t.Parallel()

		entries := []paper.Entry{
			{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1", PDFURL: "https://arxiv.org/pdf/2404.12345v1"},
			{Source: paper.SourceArxiv, SourceID: "2405.67890", Version: "", PDFURL: "https://arxiv.org/pdf/2405.67890"},
		}

		got := pdfdownload.RequestsFromEntries(entries)

		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		want0 := paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1"}
		if got[0].PaperID != want0 {
			t.Errorf("got[0].PaperID = %+v, want %+v", got[0].PaperID, want0)
		}
		if got[0].PDFURL != entries[0].PDFURL {
			t.Errorf("got[0].PDFURL = %q, want %q", got[0].PDFURL, entries[0].PDFURL)
		}
		want1 := paper.ID{Source: paper.SourceArxiv, SourceID: "2405.67890"}
		if got[1].PaperID != want1 {
			t.Errorf("got[1].PaperID = %+v, want %+v", got[1].PaperID, want1)
		}
	})

	t.Run("empty input returns a non-nil empty slice", func(t *testing.T) {
		t.Parallel()

		got := pdfdownload.RequestsFromEntries(nil)

		if got == nil {
			t.Fatal("got nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
}
