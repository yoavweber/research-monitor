package paper_test

import (
	"errors"
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

func TestIDValidate(t *testing.T) {
	t.Parallel()

	t.Run("happy path arxiv-style id with version returns no error", func(t *testing.T) {
		t.Parallel()

		id := paper.NewID(paper.SourceArxiv, "2404.12345", "v1")

		err := id.Validate()

		if err != nil {
			t.Fatalf("expected nil error for valid id, got %v", err)
		}
	})

	t.Run("happy path id without version returns no error", func(t *testing.T) {
		t.Parallel()

		id := paper.NewID(paper.SourceArxiv, "2404.12345", "")

		err := id.Validate()

		if err != nil {
			t.Fatalf("expected nil error for id with empty version, got %v", err)
		}
	})

	rejectionCases := []struct {
		name string
		id   paper.ID
	}{
		{name: "empty source is rejected", id: paper.NewID("", "2404.12345", "v1")},
		{name: "whitespace-only source is rejected", id: paper.NewID("  \t", "2404.12345", "v1")},
		{name: "empty source id is rejected", id: paper.NewID(paper.SourceArxiv, "", "v1")},
		{name: "whitespace-only source id is rejected", id: paper.NewID(paper.SourceArxiv, "\n", "v1")},
		{name: "source containing dotdot traversal is rejected", id: paper.NewID("..", "2404.12345", "v1")},
		{name: "source containing forward slash is rejected", id: paper.NewID("arxiv/evil", "2404.12345", "v1")},
		{name: "source containing backslash is rejected", id: paper.NewID("arxiv\\evil", "2404.12345", "v1")},
		{name: "source id containing dotdot traversal is rejected", id: paper.NewID(paper.SourceArxiv, "../etc/passwd", "v1")},
		{name: "source id containing forward slash is rejected", id: paper.NewID(paper.SourceArxiv, "foo/bar", "v1")},
		{name: "source id containing backslash is rejected", id: paper.NewID(paper.SourceArxiv, "foo\\bar", "v1")},
		{name: "version containing forward slash is rejected", id: paper.NewID(paper.SourceArxiv, "2404.12345", "v1/evil")},
		{name: "version containing backslash is rejected", id: paper.NewID(paper.SourceArxiv, "2404.12345", "v1\\evil")},
		{name: "version containing dotdot traversal is rejected", id: paper.NewID(paper.SourceArxiv, "2404.12345", "..")},
	}

	for _, tc := range rejectionCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.id.Validate()

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, paper.ErrInvalidID) {
				t.Fatalf("expected error to wrap ErrInvalidID, got %v", err)
			}
		})
	}
}

func TestIDPDFArtifactKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		id   paper.ID
		want string
	}{
		{
			name: "arxiv versioned id concatenates source id and version",
			id:   paper.NewID(paper.SourceArxiv, "2404.12345", "v1"),
			want: "2404.12345v1",
		},
		{
			name: "arxiv id without version returns source id only",
			id:   paper.NewID(paper.SourceArxiv, "2404.12345", ""),
			want: "2404.12345",
		},
		{
			name: "non-arxiv source uses the same concatenation rule",
			id:   paper.NewID("biorxiv", "10.1101/2023.01.01.522123", "v2"),
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

		want := paper.NewID(paper.SourceArxiv, "2404.12345", "v1")
		if got != want {
			t.Fatalf("IDFromEntry() = %+v, want %+v", got, want)
		}
	})
}
