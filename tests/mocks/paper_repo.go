package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// PaperRepoSaveResult is the per-call return tuple Save callers can preload.
// Modelling Save's two-value return as a single struct keeps caller-side
// preconfiguration aligned with how the call site reads the values back.
type PaperRepoSaveResult struct {
	IsNew bool
	Err   error
}

// PaperRepo is a hand-written paper.Repository fake for integration tests.
// It records every invocation and returns caller-configured outcomes —
// notably, Save consumes a queue of (isNew, err) tuples so a single fake
// can drive multi-entry batches through distinct branches (insert vs dedupe
// vs ErrCatalogueUnavailable) per call.
//
// Zero value is ready to use:
//   - Save returns (true, nil) once SaveResults is empty (the optimistic
//     default keeps tests that don't care about persistence outcomes terse).
//   - FindByKey and List return their respective configured fields.
//
// A mutex guards every recorded slice and the SaveResults queue so the
// fake is safe under the (single-goroutine, but conceptually concurrent)
// httptest.Server dispatch.
type PaperRepo struct {
	mu sync.Mutex

	// SaveResults is a FIFO queue of outcomes Save returns to callers, one
	// per invocation. When the queue is empty Save falls back to (true, nil)
	// — the "happy insert" default. Tests that need to fail every Save can
	// either preload enough copies or set SaveDefaultErr.
	SaveResults []PaperRepoSaveResult
	// SaveDefaultErr, when non-nil and SaveResults is exhausted, is returned
	// (with isNew=false) instead of the (true, nil) optimistic default.
	// Intended for failure-injection tests that want every Save to fail
	// regardless of how many entries the use case emits.
	SaveDefaultErr error

	// FindEntry / FindErr are the canonical FindByKey return tuple. Mutually
	// exclusive in practice: a non-nil FindErr takes precedence.
	FindEntry *paper.Entry
	FindErr   error

	// ListEntries / ListErr are the canonical List return tuple. ListErr
	// takes precedence over ListEntries.
	ListEntries []paper.Entry
	ListErr     error

	// SaveCalls captures every Entry passed to Save, in call order, so tests
	// can assert "this exact set of entries was persisted, in this order".
	SaveCalls []paper.Entry
	// FindCalls captures every (source, sourceID) tuple passed to FindByKey.
	FindCalls []PaperRepoFindCall
	// ListCalls increments on every List invocation. There is no payload
	// to record; the count alone matters for "List was called once" asserts.
	ListCalls int
}

// PaperRepoFindCall is the recorded shape of a single FindByKey invocation.
type PaperRepoFindCall struct {
	Source   string
	SourceID string
}

// Save satisfies paper.Repository. It records the entry, then pops the next
// preconfigured result; if none remain it falls back to SaveDefaultErr (when
// set) or the optimistic (true, nil) default.
func (r *PaperRepo) Save(_ context.Context, e paper.Entry) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.SaveCalls = append(r.SaveCalls, e)
	if len(r.SaveResults) > 0 {
		next := r.SaveResults[0]
		r.SaveResults = r.SaveResults[1:]
		return next.IsNew, next.Err
	}
	if r.SaveDefaultErr != nil {
		return false, r.SaveDefaultErr
	}
	return true, nil
}

// FindByKey satisfies paper.Repository.
func (r *PaperRepo) FindByKey(_ context.Context, source, sourceID string) (*paper.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.FindCalls = append(r.FindCalls, PaperRepoFindCall{Source: source, SourceID: sourceID})
	if r.FindErr != nil {
		return nil, r.FindErr
	}
	return r.FindEntry, nil
}

// List satisfies paper.Repository.
func (r *PaperRepo) List(_ context.Context) ([]paper.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ListCalls++
	if r.ListErr != nil {
		return nil, r.ListErr
	}
	return r.ListEntries, nil
}
