package paper_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

func TestIDPDFArtifactKey(t *testing.T) {
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

			got := tc.id.PDFArtifactKey()

			if got != tc.want {
				t.Fatalf("PDFArtifactKey() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIDFromEntry(t *testing.T) {
	t.Parallel()

	t.Run("copies Source SourceID Version into a fresh ID without other entry fields", func(t *testing.T) {
		t.Parallel()

		entry := paper.Entry{
			Source:   paper.SourceArxiv,
			SourceID: "2404.12345",
			Version:  "v1",
			Title:    "Some Title",
			Authors:  []string{"Alice"},
		}

		got := paper.IDFromEntry(entry)

		want := paper.ID{Source: paper.SourceArxiv, SourceID: "2404.12345", Version: "v1"}
		if got != want {
			t.Fatalf("IDFromEntry() = %+v, want %+v", got, want)
		}
	})
}
