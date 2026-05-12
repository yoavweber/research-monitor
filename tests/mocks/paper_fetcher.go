// Package mocks holds hand-written test doubles for integration tests.
package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// PaperFetcher is a hand-written paper.Fetcher fake for integration tests.
// It records every Query it receives, returns caller-configured Entries or
// Error, and counts invocations so tests can assert zero calls (e.g. when
// auth should short-circuit upstream).
//
// Zero value is ready to use: Fetch returns (nil, nil) until Entries or
// Error is set. A mutex guards recorded state so tests remain safe if the
// HTTP server ever dispatches concurrent requests.
type PaperFetcher struct {
	mu sync.Mutex

	// Entries is the slice returned to callers when Error is nil and
	// Batches is unset.
	Entries []paper.Entry
	// Batches, if non-empty, makes Fetch return Batches[invocations % len]
	// on each call. Required by concurrency tests that need every call to
	// yield a fresh batch of IsNew entries; an empty Batches falls back to
	// Entries.
	Batches [][]paper.Entry
	// Error, if non-nil, is returned in place of Entries/Batches. Intended
	// for the paper.ErrUpstream* sentinels.
	Error error

	// Queries captures every paper.Query passed to Fetch, in call order.
	Queries []paper.Query
	// Invocations is incremented on every Fetch call.
	Invocations int
}

// Fetch satisfies paper.Fetcher. It records the call, then returns either
// the configured Error, the Nth element of Batches (when set), or the
// configured Entries slice.
func (f *PaperFetcher) Fetch(_ context.Context, q paper.Query) ([]paper.Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Queries = append(f.Queries, q)
	if f.Error != nil {
		f.Invocations++
		return nil, f.Error
	}
	if len(f.Batches) > 0 {
		batch := f.Batches[f.Invocations%len(f.Batches)]
		f.Invocations++
		// Return a shallow copy so callsites that mutate per-entry fields
		// (e.g. rewriting PDFURL to point at a test server) don't leak
		// across calls.
		out := make([]paper.Entry, len(batch))
		copy(out, batch)
		return out, nil
	}
	f.Invocations++
	return f.Entries, nil
}
