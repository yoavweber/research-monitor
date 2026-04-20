package arxiv

import (
	"errors"
	"os"
	"testing"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
)

func TestParseFeed_Happy(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/happy.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	entries, err := parseFeed(body)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	e0 := entries[0]
	if e0.SourceID != "2404.12345" {
		t.Errorf("entry 0 SourceID = %q, want %q", e0.SourceID, "2404.12345")
	}
	if e0.Version != "v1" {
		t.Errorf("entry 0 Version = %q, want %q", e0.Version, "v1")
	}
	if e0.Title != "A Study on Learning Methods in Distributed Systems" {
		t.Errorf("entry 0 Title = %q", e0.Title)
	}
	if len(e0.Authors) != 1 || e0.Authors[0] != "Alice Researcher" {
		t.Errorf("entry 0 Authors = %v, want [Alice Researcher]", e0.Authors)
	}
	if e0.Abstract == "" {
		t.Errorf("entry 0 Abstract is empty")
	}
	if e0.PrimaryCategory != "cs.LG" {
		t.Errorf("entry 0 PrimaryCategory = %q, want %q", e0.PrimaryCategory, "cs.LG")
	}
	if e0.SubmittedAt.IsZero() {
		t.Errorf("entry 0 SubmittedAt is zero")
	}
	if e0.UpdatedAt.IsZero() {
		t.Errorf("entry 0 UpdatedAt is zero")
	}
	if e0.SubmittedAt.Equal(e0.UpdatedAt) {
		t.Errorf("entry 0 SubmittedAt and UpdatedAt should differ")
	}
	if wantPrefix := "http://arxiv.org/pdf/"; len(e0.PDFURL) < len(wantPrefix) || e0.PDFURL[:len(wantPrefix)] != wantPrefix {
		t.Errorf("entry 0 PDFURL = %q, should start with %q", e0.PDFURL, wantPrefix)
	}
	if wantPrefix := "http://arxiv.org/abs/"; len(e0.AbsURL) < len(wantPrefix) || e0.AbsURL[:len(wantPrefix)] != wantPrefix {
		t.Errorf("entry 0 AbsURL = %q, should start with %q", e0.AbsURL, wantPrefix)
	}

	e1 := entries[1]
	if e1.SourceID != "2403.09876" {
		t.Errorf("entry 1 SourceID = %q, want %q", e1.SourceID, "2403.09876")
	}
	if e1.Version != "v2" {
		t.Errorf("entry 1 Version = %q, want %q", e1.Version, "v2")
	}
	if len(e1.Authors) != 2 {
		t.Errorf("entry 1 Authors count = %d, want 2", len(e1.Authors))
	}
	if e1.PrimaryCategory != "q-fin.ST" {
		t.Errorf("entry 1 PrimaryCategory = %q", e1.PrimaryCategory)
	}
}

func TestParseFeed_Empty(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/empty.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	entries, err := parseFeed(body)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if entries == nil {
		t.Fatalf("parseFeed returned nil slice, want non-nil empty slice")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestParseFeed_Malformed(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/malformed.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	entries, err := parseFeed(body)
	if err == nil {
		t.Fatalf("parseFeed succeeded on malformed input, entries=%d", len(entries))
	}
	if !errors.Is(err, paper.ErrUpstreamMalformed) {
		t.Errorf("expected err Is paper.ErrUpstreamMalformed, got %v", err)
	}
}

func TestParseFeed_ErrorEntry(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/error_entry.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	entries, err := parseFeed(body)
	if err == nil {
		t.Fatalf("parseFeed succeeded on error-entry feed, entries=%d", len(entries))
	}
	if !errors.Is(err, paper.ErrUpstreamMalformed) {
		t.Errorf("expected err Is paper.ErrUpstreamMalformed, got %v", err)
	}
}
