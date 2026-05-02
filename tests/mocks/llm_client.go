package mocks

import (
	"context"
	"fmt"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// LLMResult bundles a (response, error) pair so a test can script Complete
// outcomes one call at a time. Exactly one of Response or Err should be set
// per entry; if both are zero, Complete returns a minimal default response.
type LLMResult struct {
	Response *shared.LLMResponse
	Err      error
}

// LLMClientFake is a hand-written shared.LLMClient double for use-case tests.
// Tests script per-call behavior by appending to Results before calling the
// use case. Each Complete invocation pops the next Results entry; when the
// queue is empty Complete returns a minimal default response so a test that
// only cares about a subset of calls does not need to script every one.
//
// All recorded state is mutex-guarded so the test can read CallCount / Calls
// after the use case returns without racing against in-flight Complete calls.
type LLMClientFake struct {
	mu sync.Mutex

	Results []LLMResult

	Calls     []shared.LLMRequest
	CallCount int
}

// QueueResponse appends a successful response to the script.
func (f *LLMClientFake) QueueResponse(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Results = append(f.Results, LLMResult{Response: &shared.LLMResponse{Text: text, Model: "fake-test"}})
}

// QueueError appends a transport-style error to the script. Subsequent
// Complete calls past this point pop the next entry as usual.
func (f *LLMClientFake) QueueError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Results = append(f.Results, LLMResult{Err: err})
}

// Complete is the shared.LLMClient implementation. The first queued result is
// returned and removed; if the queue is empty a generic placeholder response
// is returned so callers don't have to script every prompt.
func (f *LLMClientFake) Complete(_ context.Context, req shared.LLMRequest) (*shared.LLMResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.CallCount++
	f.Calls = append(f.Calls, req)

	if len(f.Results) == 0 {
		return &shared.LLMResponse{
			Text:          fmt.Sprintf("default-fake-response-for-%s", req.PromptVersion),
			Model:         "fake-test",
			PromptVersion: req.PromptVersion,
		}, nil
	}

	next := f.Results[0]
	f.Results = f.Results[1:]
	if next.Err != nil {
		return nil, next.Err
	}
	if next.Response == nil {
		return &shared.LLMResponse{}, nil
	}
	resp := *next.Response
	if resp.PromptVersion == "" {
		resp.PromptVersion = req.PromptVersion
	}
	return &resp, nil
}

// Snapshot returns a defensive copy of the recorded calls so a test can
// inspect them without holding the fake's lock.
func (f *LLMClientFake) Snapshot() []shared.LLMRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]shared.LLMRequest, len(f.Calls))
	copy(out, f.Calls)
	return out
}
