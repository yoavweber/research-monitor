package source

import "context"

// Repository — persistence port. Implemented in
// infrastructure/persistence/source/repo.go.
type Repository interface {
	Save(ctx context.Context, s *Source) error
	FindByID(ctx context.Context, id string) (*Source, error)
	FindByURL(ctx context.Context, url string) (*Source, error)
	List(ctx context.Context) ([]Source, error)
	Delete(ctx context.Context, id string) error
}

// UseCase — application port. Implemented in
// application/source_usecase.go.
type UseCase interface {
	Create(ctx context.Context, req CreateRequest) (*Source, error)
	Get(ctx context.Context, id string) (*Source, error)
	List(ctx context.Context) ([]Source, error)
	Update(ctx context.Context, id string, req UpdateRequest) (*Source, error)
	Delete(ctx context.Context, id string) error
}
