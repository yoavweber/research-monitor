package local_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/httpclient"
	pdflocal "github.com/yoavweber/research-monitor/backend/internal/infrastructure/pdf/local"
	"github.com/yoavweber/research-monitor/backend/tests/mocks"
)

// TestStore_Ensure_RealHTTPFetcher wires the real httpclient byte fetcher
// against an httptest.Server so the local store's fetch+atomic-write+cache
// gate is exercised end-to-end through actual HTTP and filesystem I/O —
// not against the unit-test fake. This pins down the cross-package contract
// the unit-level tests in store_test.go cannot: that the production fetcher
// surfaces shared.ErrBadStatus and context.DeadlineExceeded in the exact
// shape the store wraps under pdf.ErrFetch.
func TestStore_Ensure_RealHTTPFetcher(t *testing.T) {
	t.Parallel()

	t.Run("cache miss followed by cache hit makes exactly one request", func(t *testing.T) {
		t.Parallel()

		// Distinguishable but small: enough to verify byte-for-byte equality
		// without paying the cost of a real PDF fixture in repo.
		wantBody := []byte("%PDF-1.4\nstub-bytes-for-integration-test\n%%EOF\n")

		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(wantBody)
		}))
		t.Cleanup(srv.Close)

		fetcher := httpclient.NewByteFetcher(2*time.Second, "integration-test/1.0")
		logger := &mocks.RecordingLogger{}
		store, err := pdflocal.NewStore(t.TempDir(), fetcher, logger)
		if err != nil {
			t.Fatalf("NewStore returned error: %v", err)
		}

		key := pdf.Key{SourceType: "paper", SourceID: "integration-cache-1", URL: srv.URL + "/papers/x.pdf"}

		loc, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure (miss) returned error: %v", err)
		}
		if loc == nil {
			t.Fatal("Ensure (miss) returned nil locator")
		}
		gotBytes, err := os.ReadFile(loc.Path())
		if err != nil {
			t.Fatalf("read canonical file after miss: %v", err)
		}
		if !bytes.Equal(gotBytes, wantBody) {
			t.Fatalf("file bytes after miss = %q, want %q", gotBytes, wantBody)
		}
		if got := hits.Load(); got != 1 {
			t.Fatalf("hit counter after miss = %d, want 1", got)
		}

		loc2, err := store.Ensure(context.Background(), key)

		if err != nil {
			t.Fatalf("Ensure (hit) returned error: %v", err)
		}
		if loc2 == nil {
			t.Fatal("Ensure (hit) returned nil locator")
		}
		gotBytes2, err := os.ReadFile(loc2.Path())
		if err != nil {
			t.Fatalf("read canonical file after hit: %v", err)
		}
		if !bytes.Equal(gotBytes2, wantBody) {
			t.Fatalf("file bytes after hit = %q, want %q", gotBytes2, wantBody)
		}
		if got := hits.Load(); got != 1 {
			t.Fatalf("hit counter after cache-hit Ensure = %d, want 1 (cache must not re-fetch)", got)
		}
	})

	t.Run("non-2xx status surfaces ErrFetch chained with ErrBadStatus", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		fetcher := httpclient.NewByteFetcher(2*time.Second, "integration-test/1.0")
		logger := &mocks.RecordingLogger{}
		root := t.TempDir()
		store, err := pdflocal.NewStore(root, fetcher, logger)
		if err != nil {
			t.Fatalf("NewStore returned error: %v", err)
		}

		key := pdf.Key{SourceType: "paper", SourceID: "integration-404", URL: srv.URL + "/missing.pdf"}

		loc, err := store.Ensure(context.Background(), key)

		if err == nil {
			t.Fatal("Ensure returned nil error, want non-2xx fetch error")
		}
		if loc != nil {
			t.Fatalf("Ensure returned non-nil locator on error: %v", loc)
		}
		if !errors.Is(err, pdf.ErrFetch) {
			t.Fatalf("errors.Is(err, pdf.ErrFetch) = false, want true; err = %v", err)
		}
		if !errors.Is(err, shared.ErrBadStatus) {
			t.Fatalf("errors.Is(err, shared.ErrBadStatus) = false, want true; err = %v", err)
		}

		// No canonical file should exist on a fetch failure.
		canonical := root + "/paper/integration-404.pdf"
		if _, statErr := os.Stat(canonical); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("os.Stat(canonical) err = %v, want fs.ErrNotExist (no file should be created on fetch failure)", statErr)
		}
	})

	t.Run("deadline-bound context surfaces ErrFetch chained with DeadlineExceeded", func(t *testing.T) {
		t.Parallel()

		// Handler stays responsive to client cancellation: the request
		// context fires first (50ms) and the handler observes it via
		// r.Context().Done(), so the test does not depend on the 500ms
		// fallback ever elapsing.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(500 * time.Millisecond):
				w.WriteHeader(http.StatusOK)
			case <-r.Context().Done():
				return
			}
		}))
		t.Cleanup(srv.Close)

		// Long fetcher-level timeout so the per-request context deadline
		// (set below) is the cause of the cancellation, not the transport
		// timeout — that's what makes the asserted DeadlineExceeded
		// identification meaningful.
		fetcher := httpclient.NewByteFetcher(2*time.Second, "integration-test/1.0")
		logger := &mocks.RecordingLogger{}
		root := t.TempDir()
		store, err := pdflocal.NewStore(root, fetcher, logger)
		if err != nil {
			t.Fatalf("NewStore returned error: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		key := pdf.Key{SourceType: "paper", SourceID: "integration-deadline", URL: srv.URL + "/slow.pdf"}

		loc, err := store.Ensure(ctx, key)

		if err == nil {
			t.Fatal("Ensure returned nil error, want deadline error")
		}
		if loc != nil {
			t.Fatalf("Ensure returned non-nil locator on error: %v", loc)
		}
		if !errors.Is(err, pdf.ErrFetch) {
			t.Fatalf("errors.Is(err, pdf.ErrFetch) = false, want true; err = %v", err)
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("errors.Is(err, context.DeadlineExceeded) = false, want true; err = %v", err)
		}

		canonical := root + "/paper/integration-deadline.pdf"
		if _, statErr := os.Stat(canonical); !errors.Is(statErr, fs.ErrNotExist) {
			t.Fatalf("os.Stat(canonical) err = %v, want fs.ErrNotExist (no file should be created on deadline)", statErr)
		}
	})
}
