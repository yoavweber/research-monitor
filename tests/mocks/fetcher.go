// Package mocks holds hand-written test doubles for integration tests.
package mocks

import (
	"context"
	"sync"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Compile-time conformance — surfaces shared.Fetcher port drift at build
// time rather than at first use from a test.
var _ shared.Fetcher = (*Fetcher)(nil)

// Fetcher is a hand-written shared.Fetcher fake for unit tests. It mirrors
// the structure of PaperFetcher in this package: zero value is ready to
// use, a sync.Mutex guards recorded state, exported fields configure the
// response, and recorded URLs plus an invocation counter let tests assert
// call patterns (including zero-call expectations such as cache hits).
//
// The Sleep field exists so cancellation tests can simulate a slow
// upstream and observe ctx.Done() promptly without resorting to real
// network sockets. Fetch honours ctx.Done() during the sleep so tests
// remain deterministic even if the test machine is under load.
type Fetcher struct {
	mu sync.Mutex

	// Body is the byte slice returned when Error is nil. Fetch returns a
	// fresh copy so callers cannot mutate the configured response by
	// writing into the slice.
	Body []byte
	// Error, if non-nil, is returned in place of Body.
	Error error
	// Sleep, if non-zero, blocks Fetch for that duration before returning.
	// The wait is interrupted by ctx cancellation so cancellation tests
	// can pair Sleep with a deadline or AfterFunc cancel.
	Sleep time.Duration

	// URLs captures every URL passed to Fetch, in call order.
	URLs []string
	// Invocations is incremented on every Fetch call, regardless of
	// outcome (including ctx-cancellation paths).
	Invocations int
}

// Fetch satisfies shared.Fetcher.
func (f *Fetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	f.Invocations++
	f.URLs = append(f.URLs, url)
	sleep := f.Sleep
	body := f.Body
	err := f.Error
	f.mu.Unlock()

	if sleep > 0 {
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	// Defensive copy so test assertions on the returned slice cannot
	// observe mutations a later Fetch call might make to Body.
	out := make([]byte, len(body))
	copy(out, body)
	return out, nil
}
