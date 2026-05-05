package mocks

import (
	"context"
	"sync"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

// InMemoryAnalyzerRepo is a tiny analyzer.Repository for unit tests.
// Persistence-level upsert semantics (race-safe overwrite, created_at
// preservation) are exercised against a real DB in the persistence
// package's tests; this fake only needs to round-trip a single row per id.
type InMemoryAnalyzerRepo struct {
	mu       sync.Mutex
	Rows     map[string]domain.Analysis
	UpsertEr error
	FindErr  error
	Upserts  int
}

func NewInMemoryAnalyzerRepo() *InMemoryAnalyzerRepo {
	return &InMemoryAnalyzerRepo{Rows: map[string]domain.Analysis{}}
}

func (r *InMemoryAnalyzerRepo) Upsert(_ context.Context, a domain.Analysis) (domain.Analysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Upserts++
	if r.UpsertEr != nil {
		return domain.Analysis{}, r.UpsertEr
	}
	if prior, ok := r.Rows[a.ExtractionID]; ok {
		a.CreatedAt = prior.CreatedAt
	}
	r.Rows[a.ExtractionID] = a
	return a, nil
}

func (r *InMemoryAnalyzerRepo) FindByID(_ context.Context, id string) (*domain.Analysis, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FindErr != nil {
		return nil, r.FindErr
	}
	row, ok := r.Rows[id]
	if !ok {
		return nil, domain.ErrAnalysisNotFound
	}
	return &row, nil
}
