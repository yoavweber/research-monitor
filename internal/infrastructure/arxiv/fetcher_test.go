package arxiv

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

// fakeFetcher is an inline stub of shared.Fetcher. It captures the URL it was
// invoked with, counts invocations, and returns configurable body/error values.
type fakeFetcher struct {
	receivedURL string
	invocations int
	returnBody  []byte
	returnErr   error
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	f.invocations++
	f.receivedURL = url
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return f.returnBody, nil
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return b
}

const testBaseURL = "https://export.arxiv.org/api/query"

func TestArxivFetcher_URL_SingleCategory(t *testing.T) {
	t.Parallel()

	body := mustRead(t, "testdata/empty.xml")
	fake := &fakeFetcher{returnBody: body}

	f := NewArxivFetcher(testBaseURL, fake)
	_, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	got := fake.receivedURL
	if !strings.HasPrefix(got, testBaseURL+"?") {
		t.Errorf("URL prefix mismatch: %q", got)
	}
	if !strings.Contains(got, "search_query=cat:cs.LG") {
		t.Errorf("expected search_query=cat:cs.LG in %q", got)
	}
	if strings.Contains(got, "%28") || strings.Contains(got, "%29") {
		t.Errorf("single-category URL must not contain encoded parens, got %q", got)
	}
	if !strings.Contains(got, "max_results=100") {
		t.Errorf("missing max_results=100 in %q", got)
	}
	if !strings.Contains(got, "sortBy=submittedDate") {
		t.Errorf("missing sortBy=submittedDate in %q", got)
	}
	if !strings.Contains(got, "sortOrder=descending") {
		t.Errorf("missing sortOrder=descending in %q", got)
	}
}

func TestArxivFetcher_URL_MultiCategory_OR_WithParens(t *testing.T) {
	t.Parallel()

	body := mustRead(t, "testdata/empty.xml")
	fake := &fakeFetcher{returnBody: body}

	f := NewArxivFetcher(testBaseURL, fake)
	_, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG", "q-fin.ST"},
		MaxResults: 50,
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	got := fake.receivedURL
	if !strings.Contains(got, "search_query=%28cat:cs.LG+OR+cat:q-fin.ST%29") {
		t.Errorf("multi-category URL shape mismatch: %q", got)
	}
	if !strings.Contains(got, "max_results=50") {
		t.Errorf("missing max_results=50 in %q", got)
	}
	if !strings.Contains(got, "sortBy=submittedDate") {
		t.Errorf("missing sortBy=submittedDate in %q", got)
	}
	if !strings.Contains(got, "sortOrder=descending") {
		t.Errorf("missing sortOrder=descending in %q", got)
	}
}

func TestArxivFetcher_Success_ReturnsEntries(t *testing.T) {
	t.Parallel()

	body := mustRead(t, "testdata/happy.xml")
	fake := &fakeFetcher{returnBody: body}

	f := NewArxivFetcher(testBaseURL, fake)
	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SourceID != "2404.12345" {
		t.Errorf("entries[0].SourceID = %q, want %q", entries[0].SourceID, "2404.12345")
	}
}

func TestArxivFetcher_EmptyFeed_IsSuccess(t *testing.T) {
	t.Parallel()

	body := mustRead(t, "testdata/empty.xml")
	fake := &fakeFetcher{returnBody: body}

	f := NewArxivFetcher(testBaseURL, fake)
	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if entries == nil {
		t.Fatalf("expected non-nil empty slice, got nil")
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestArxivFetcher_BadStatus_MapsTo_ErrUpstreamBadStatus(t *testing.T) {
	t.Parallel()

	fake := &fakeFetcher{returnErr: fmt.Errorf("%w: status=500", shared.ErrBadStatus)}
	f := NewArxivFetcher(testBaseURL, fake)

	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if !errors.Is(err, paper.ErrUpstreamBadStatus) {
		t.Fatalf("expected ErrUpstreamBadStatus, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries on error, got %v", entries)
	}
}

func TestArxivFetcher_DeadlineExceeded_MapsTo_ErrUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	fake := &fakeFetcher{returnErr: context.DeadlineExceeded}
	f := NewArxivFetcher(testBaseURL, fake)

	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if !errors.Is(err, paper.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries on error, got %v", entries)
	}
}

func TestArxivFetcher_NetworkError_MapsTo_ErrUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	urlErr := &url.Error{
		Op:  "Get",
		URL: "http://example.invalid/",
		Err: errors.New("dial tcp: connection refused"),
	}
	fake := &fakeFetcher{returnErr: urlErr}
	f := NewArxivFetcher(testBaseURL, fake)

	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if !errors.Is(err, paper.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries on error, got %v", entries)
	}
}

func TestArxivFetcher_UnknownError_MapsTo_ErrUpstreamUnavailable(t *testing.T) {
	t.Parallel()

	fake := &fakeFetcher{returnErr: errors.New("something weird")}
	f := NewArxivFetcher(testBaseURL, fake)

	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if !errors.Is(err, paper.ErrUpstreamUnavailable) {
		t.Fatalf("expected ErrUpstreamUnavailable fallback, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries on error, got %v", entries)
	}
}

func TestArxivFetcher_MalformedBody_MapsTo_ErrUpstreamMalformed(t *testing.T) {
	t.Parallel()

	body := mustRead(t, "testdata/malformed.xml")
	fake := &fakeFetcher{returnBody: body}

	f := NewArxivFetcher(testBaseURL, fake)
	entries, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG"},
		MaxResults: 100,
	})
	if !errors.Is(err, paper.ErrUpstreamMalformed) {
		t.Fatalf("expected ErrUpstreamMalformed, got %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries on error, got %v", entries)
	}
}

func TestArxivFetcher_SingleOutboundCall(t *testing.T) {
	t.Parallel()

	body := mustRead(t, "testdata/empty.xml")
	fake := &fakeFetcher{returnBody: body}

	f := NewArxivFetcher(testBaseURL, fake)
	_, err := f.Fetch(context.Background(), paper.Query{
		Categories: []string{"cs.LG", "q-fin.ST"},
		MaxResults: 100,
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if fake.invocations != 1 {
		t.Fatalf("expected exactly 1 outbound call, got %d", fake.invocations)
	}
}
