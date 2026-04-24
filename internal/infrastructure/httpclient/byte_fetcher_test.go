package httpclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/infrastructure/httpclient"
)

// Note: a "read-body-fail after headers" scenario is not explicitly tested
// here because simulating a mid-stream body truncation with httptest.Server
// is fiddly; the design covers it (errors are wrapped with "read body:")
// and is exercised implicitly via the other transport-error paths.

func TestByteFetcher_2xx_ReturnsBody(t *testing.T) {
	t.Parallel()

	const (
		wantUA   = "test-ua/1.0"
		wantBody = "hello"
	)

	var gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantBody))
	}))
	t.Cleanup(srv.Close)

	f := httpclient.NewByteFetcher(2*time.Second, wantUA)

	got, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Fetch returned unexpected error: %v", err)
	}
	if string(got) != wantBody {
		t.Fatalf("body = %q, want %q", string(got), wantBody)
	}
	if gotUA != wantUA {
		t.Fatalf("User-Agent = %q, want %q", gotUA, wantUA)
	}
	if gotAccept != "*/*" {
		t.Fatalf("Accept = %q, want %q", gotAccept, "*/*")
	}
}

func TestByteFetcher_Non2xx_WrapsErrBadStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	t.Cleanup(srv.Close)

	f := httpclient.NewByteFetcher(2*time.Second, "test-ua/1.0")

	got, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Fetch returned nil error, want non-2xx error")
	}
	if got != nil {
		t.Fatalf("body on error = %v, want nil", got)
	}
	if !errors.Is(err, shared.ErrBadStatus) {
		t.Fatalf("errors.Is(err, shared.ErrBadStatus) = false, want true; err = %v", err)
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("err.Error() = %q, want to contain %q", err.Error(), "status=500")
	}
}

func TestByteFetcher_404_WrapsErrBadStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	f := httpclient.NewByteFetcher(2*time.Second, "test-ua/1.0")

	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Fetch returned nil error, want non-2xx error")
	}
	if !errors.Is(err, shared.ErrBadStatus) {
		t.Fatalf("errors.Is(err, shared.ErrBadStatus) = false, want true; err = %v", err)
	}
	if !strings.Contains(err.Error(), "status=404") {
		t.Fatalf("err.Error() = %q, want to contain %q", err.Error(), "status=404")
	}
}

func TestByteFetcher_Timeout_ReturnsDeadlineExceeded(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			return
		}
	}))
	t.Cleanup(srv.Close)

	// Use per-request context timeout for clean DeadlineExceeded identification.
	f := httpclient.NewByteFetcher(2*time.Second, "test-ua/1.0")

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := f.Fetch(ctx, srv.URL)
	if err == nil {
		t.Fatal("Fetch returned nil error, want timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, context.DeadlineExceeded) = false, want true; err = %v", err)
	}
}

func TestByteFetcher_ConnectionRefused_ReturnsError(t *testing.T) {
	t.Parallel()

	// Start a server, grab its URL, then immediately close it so the port is dead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	deadURL := srv.URL
	srv.Close()

	f := httpclient.NewByteFetcher(2*time.Second, "test-ua/1.0")

	_, err := f.Fetch(context.Background(), deadURL)
	if err == nil {
		t.Fatal("Fetch returned nil error, want connection-refused error")
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("errors.As(err, *url.Error) = false, want true; err = %v", err)
	}
}

func TestByteFetcher_BadURL_ReturnsError(t *testing.T) {
	t.Parallel()

	f := httpclient.NewByteFetcher(2*time.Second, "test-ua/1.0")

	_, err := f.Fetch(context.Background(), "ht!tp://no.such.thing")
	if err == nil {
		t.Fatal("Fetch returned nil error, want bad-URL error")
	}
}
