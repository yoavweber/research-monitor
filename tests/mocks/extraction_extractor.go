package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ExtractorFake is a hand-written extraction.Extractor fake for unit tests.
// It records every ExtractInput it receives, returns caller-configured Output
// or Err, and exposes a BlockUntil channel so tests that need to observe an
// in-flight extraction (notably the worker shutdown test in Task 3.4) can
// pin Extract on a deterministic synchronization point.
//
// Zero value is ready to use: Extract returns the zero ExtractOutput with no
// error until Output or Err is set. A mutex guards recorded state for safety
// when tests dispatch concurrent calls.
type ExtractorFake struct {
	mu sync.Mutex

	// Output is returned to callers when Err is nil.
	Output extraction.ExtractOutput
	// Err, when non-nil, is returned in place of Output. Tests typically set
	// this to one of the extraction.ErrScannedPDF / ErrParseFailed /
	// ErrExtractorFailure sentinels, or to context.Canceled.
	Err error

	// BlockUntil, if non-nil, causes Extract to receive from this channel
	// before returning. Tests close (or send on) the channel to release the
	// in-flight call. Used by the Task 3.4 worker shutdown test to deterministically
	// freeze a Process call mid-flight.
	BlockUntil chan struct{}

	// Calls captures every ExtractInput passed to Extract, in call order.
	Calls []extraction.ExtractInput
	// CallCount is incremented on every Extract call.
	CallCount int
}

// Extract satisfies extraction.Extractor. It records the call, optionally
// blocks on BlockUntil, and returns the configured Output / Err.
func (f *ExtractorFake) Extract(ctx context.Context, in extraction.ExtractInput) (extraction.ExtractOutput, error) {
	f.mu.Lock()
	f.CallCount++
	f.Calls = append(f.Calls, in)
	block := f.BlockUntil
	out := f.Output
	err := f.Err
	f.mu.Unlock()

	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return extraction.ExtractOutput{}, ctx.Err()
		}
	}

	if err != nil {
		return extraction.ExtractOutput{}, err
	}
	return out, nil
}

// RecordedCalls returns a copy of the recorded ExtractInput slice under lock,
// so callers can iterate without racing against an in-flight Extract.
func (f *ExtractorFake) RecordedCalls() []extraction.ExtractInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]extraction.ExtractInput, len(f.Calls))
	copy(out, f.Calls)
	return out
}
