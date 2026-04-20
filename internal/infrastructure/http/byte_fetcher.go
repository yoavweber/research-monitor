// Package http provides a concrete, source-neutral implementation of the
// shared.Fetcher port: a byte-level HTTP GET client that performs a single
// request per call, returns the response body on 2xx, wraps shared.ErrBadStatus
// on non-2xx, and surfaces stdlib-identifiable transport errors otherwise.
//
// The package is named "http" (matching its directory) and clashes with the
// stdlib "net/http" package; callers that need both should alias this one
// (e.g. `import httpinfra ".../internal/infrastructure/http"`).
package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"time"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

// byteFetcher is the concrete shared.Fetcher implementation. It holds a
// long-lived *http.Client so connection pooling survives across calls.
type byteFetcher struct {
	client    *stdhttp.Client
	userAgent string
}

// NewByteFetcher returns a shared.Fetcher performing single HTTP GETs with the
// configured timeout and User-Agent. It is goroutine-safe.
//
// Preconditions (trusted from the caller / bootstrap): timeout > 0 and
// userAgent non-empty. The constructor does not panic on violations.
func NewByteFetcher(timeout time.Duration, userAgent string) shared.Fetcher {
	return &byteFetcher{
		client: &stdhttp.Client{
			Timeout: timeout,
			// No custom redirect policy: Go's default (follow up to 10
			// redirects) is intentional for generic use.
		},
		userAgent: userAgent,
	}
}

// Fetch performs a single GET against the provided URL. It returns the raw
// response body on 2xx. On non-2xx, it returns an error wrapping
// shared.ErrBadStatus with the received status code. On transport failures,
// it surfaces stdlib errors (context.DeadlineExceeded, *url.Error, ...) so
// adapters above this layer can classify them.
func (f *byteFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Best-effort drain for connection reuse; drain errors are ignored
		// because the status-code error is already definitive.
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("%w: status=%d", shared.ErrBadStatus, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return body, nil
}
