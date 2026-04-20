package paper

import "context"

// Query is the immutable, source-neutral fetch criterion passed to any
// paper.Fetcher implementation. Validation (non-empty Categories, MaxResults
// within the allowed range) is enforced by the bootstrap layer at
// construction time; the struct itself does not re-validate.
type Query struct {
	Categories []string
	MaxResults int
}

// UseCase — application port consumed by the HTTP controller. Implemented in
// application/arxiv/usecase.go.
type UseCase interface {
	Fetch(ctx context.Context) ([]Entry, error)
}

// Fetcher — source-neutral domain-level fetch port. Implementations translate
// a Query into a source-specific call, execute it, and return typed Entry
// values or a paper.* sentinel. Concrete impls live in infrastructure/<source>/.
type Fetcher interface {
	Fetch(ctx context.Context, q Query) ([]Entry, error)
}
