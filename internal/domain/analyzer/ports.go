package analyzer

import "context"

// UseCase is the inbound port the HTTP controller consumes. Analyze runs the
// synchronous three-call orchestration and returns the freshly persisted
// row; Get is a read-only retrieval that never invokes the LLM.
type UseCase interface {
	Analyze(ctx context.Context, extractionID string) (*Analysis, error)
	Get(ctx context.Context, extractionID string) (*Analysis, error)
}

// Repository is the outbound persistence port. Upsert preserves CreatedAt on
// overwrite and advances UpdatedAt; FindByID returns ErrAnalysisNotFound for
// misses and wraps any other storage failure as ErrCatalogueUnavailable.
type Repository interface {
	Upsert(ctx context.Context, a Analysis) (Analysis, error)
	FindByID(ctx context.Context, extractionID string) (*Analysis, error)
}
