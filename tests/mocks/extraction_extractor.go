package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ExtractorFake is a hand-written extraction.Extractor fake. Zero value is
// usable: Extract returns the zero ExtractOutput with nil err until Output /
// Err is set. BlockUntil, when non-nil, gates Extract on a channel receive so
// tests can observe an in-flight extraction at a deterministic point — the
// worker shutdown test relies on this. Mutex-guarded so concurrent test
// dispatches don't race against recorded state.
type ExtractorFake struct {
	mu sync.Mutex

	Output     extraction.ExtractOutput
	Err        error
	BlockUntil chan struct{}

	Calls     []extraction.ExtractInput
	CallCount int
}

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

func (f *ExtractorFake) RecordedCalls() []extraction.ExtractInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]extraction.ExtractInput, len(f.Calls))
	copy(out, f.Calls)
	return out
}
