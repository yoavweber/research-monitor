package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ExtractionUseCaseFake is a hand-written extraction.UseCase fake. Zero value
// is usable: each method returns its zero-valued result until the caller sets
// the corresponding Result / Err field. CallsSnapshot returns a defensive copy
// so concurrent test dispatches don't race against the recorded log.
type ExtractionUseCaseFake struct {
	mu sync.Mutex

	SubmitResult extraction.SubmitResult
	SubmitErr    error

	GetResult *extraction.Extraction
	GetErr    error

	ProcessErr error

	Calls struct {
		Submit  []extraction.RequestPayload
		Get     []string
		Process []extraction.Extraction
	}
}

func (f *ExtractionUseCaseFake) Submit(_ context.Context, p extraction.RequestPayload) (extraction.SubmitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls.Submit = append(f.Calls.Submit, p)
	if f.SubmitErr != nil {
		return extraction.SubmitResult{}, f.SubmitErr
	}
	return f.SubmitResult, nil
}

func (f *ExtractionUseCaseFake) Get(_ context.Context, id string) (*extraction.Extraction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls.Get = append(f.Calls.Get, id)
	if f.GetErr != nil {
		return nil, f.GetErr
	}
	return f.GetResult, nil
}

func (f *ExtractionUseCaseFake) Process(_ context.Context, row extraction.Extraction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls.Process = append(f.Calls.Process, row)
	return f.ProcessErr
}

func (f *ExtractionUseCaseFake) CallsSnapshot() (submit []extraction.RequestPayload, get []string, process []extraction.Extraction) {
	f.mu.Lock()
	defer f.mu.Unlock()
	submit = append([]extraction.RequestPayload(nil), f.Calls.Submit...)
	get = append([]string(nil), f.Calls.Get...)
	process = append([]extraction.Extraction(nil), f.Calls.Process...)
	return submit, get, process
}
