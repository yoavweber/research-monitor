package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
)

// ExtractionUseCaseFake is a hand-written extraction.UseCase fake for
// integration and controller-level tests. Every method records its call
// under a mutex and returns the caller-configured queued response — the
// shape mirrors PaperFetcher / PaperRepo so tests across packages share a
// single mental model.
//
// Zero value is ready to use: each method returns its zero-valued result
// until the caller assigns a Result / Err field.
type ExtractionUseCaseFake struct {
	mu sync.Mutex

	// SubmitResult / SubmitErr is the canonical Submit return tuple.
	// SubmitErr takes precedence when non-nil.
	SubmitResult extraction.SubmitResult
	SubmitErr    error

	// GetResult / GetErr is the canonical Get return tuple. GetErr takes
	// precedence when non-nil; otherwise GetResult is returned as-is
	// (callers can preload nil to model "found nothing without an error",
	// though the production contract is to return ErrNotFound instead).
	GetResult *extraction.Extraction
	GetErr    error

	// ProcessErr is the error returned by Process. nil signals success.
	ProcessErr error

	// Calls captures every invocation in call order. Tests read it via
	// CallsSnapshot to stay race-free under concurrent dispatch.
	Calls struct {
		Submit  []extraction.RequestPayload
		Get     []string
		Process []extraction.Extraction
	}
}

// Submit satisfies extraction.UseCase. It records the payload and returns
// the configured (SubmitResult, SubmitErr) tuple.
func (f *ExtractionUseCaseFake) Submit(_ context.Context, p extraction.RequestPayload) (extraction.SubmitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls.Submit = append(f.Calls.Submit, p)
	if f.SubmitErr != nil {
		return extraction.SubmitResult{}, f.SubmitErr
	}
	return f.SubmitResult, nil
}

// Get satisfies extraction.UseCase. It records the id and returns the
// configured (GetResult, GetErr) tuple.
func (f *ExtractionUseCaseFake) Get(_ context.Context, id string) (*extraction.Extraction, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls.Get = append(f.Calls.Get, id)
	if f.GetErr != nil {
		return nil, f.GetErr
	}
	return f.GetResult, nil
}

// Process satisfies extraction.UseCase. It records the row and returns
// ProcessErr.
func (f *ExtractionUseCaseFake) Process(_ context.Context, row extraction.Extraction) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls.Process = append(f.Calls.Process, row)
	return f.ProcessErr
}

// CallsSnapshot returns a copy of the recorded call log under lock. Tests
// inspect this instead of reading Calls directly so the read sees a
// consistent slice even if a parallel handler is recording mid-assertion.
func (f *ExtractionUseCaseFake) CallsSnapshot() (submit []extraction.RequestPayload, get []string, process []extraction.Extraction) {
	f.mu.Lock()
	defer f.mu.Unlock()
	submit = append([]extraction.RequestPayload(nil), f.Calls.Submit...)
	get = append([]string(nil), f.Calls.Get...)
	process = append([]extraction.Extraction(nil), f.Calls.Process...)
	return submit, get, process
}
