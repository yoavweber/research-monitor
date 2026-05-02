package mocks

import (
	"context"
	"fmt"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

type LLMResult struct {
	Response *shared.LLMResponse
	Err      error
}

// LLMClientFake is a programmable shared.LLMClient. Each Complete pops the
// next Results entry; an empty queue returns a placeholder so tests that
// only care about a subset of calls don't have to script every prompt.
type LLMClientFake struct {
	mu sync.Mutex

	Results []LLMResult

	Calls     []shared.LLMRequest
	CallCount int
}

func (f *LLMClientFake) QueueResponse(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Results = append(f.Results, LLMResult{Response: &shared.LLMResponse{Text: text, Model: "fake-test"}})
}

func (f *LLMClientFake) QueueError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Results = append(f.Results, LLMResult{Err: err})
}

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

func (f *LLMClientFake) Snapshot() []shared.LLMRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]shared.LLMRequest, len(f.Calls))
	copy(out, f.Calls)
	return out
}
