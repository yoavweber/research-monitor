package analyzer

import "context"

type UseCase interface {
	Analyze(ctx context.Context, extractionID string) (*Analysis, error)
	Get(ctx context.Context, extractionID string) (*Analysis, error)
}

// Repository preserves CreatedAt on overwrite and advances UpdatedAt;
// FindByID returns ErrAnalysisNotFound for misses and wraps any other
// storage failure as ErrCatalogueUnavailable.
type Repository interface {
	Upsert(ctx context.Context, a Analysis) (Analysis, error)
	FindByID(ctx context.Context, extractionID string) (*Analysis, error)
}
