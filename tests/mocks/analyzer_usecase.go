package mocks

import (
	"context"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

// AnalyzerUseCaseFake is a programmable analyzer.UseCase double. AnalyzeFn /
// GetFn are mandatory for the cases under test — unset methods panic so a
// stray call surfaces loudly rather than returning a zero value.
type AnalyzerUseCaseFake struct {
	AnalyzeFn func(ctx context.Context, id string) (*domain.Analysis, error)
	GetFn     func(ctx context.Context, id string) (*domain.Analysis, error)

	AnalyzeCalls int
	GetCalls     int
}

func (f *AnalyzerUseCaseFake) Analyze(ctx context.Context, id string) (*domain.Analysis, error) {
	f.AnalyzeCalls++
	if f.AnalyzeFn == nil {
		panic("Analyze called but no AnalyzeFn programmed")
	}
	return f.AnalyzeFn(ctx, id)
}

func (f *AnalyzerUseCaseFake) Get(ctx context.Context, id string) (*domain.Analysis, error) {
	f.GetCalls++
	if f.GetFn == nil {
		panic("Get called but no GetFn programmed")
	}
	return f.GetFn(ctx, id)
}
