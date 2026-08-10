package paper_test

import (
	"testing"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

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
