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

// Fetcher — source-neutral domain-level fetch port. Implementations translate
// a Query into a source-specific call, execute it, and return typed Entry
// values or a paper.* sentinel. Concrete impls live in infrastructure/<source>/.
type Fetcher interface {
	Fetch(ctx context.Context, q Query) ([]Entry, error)
}

// Repository — persistence port for Entry values. Implementations live in
// infrastructure/<storage>/. The port is source-neutral; callers supply the
// (Source, SourceID) composite key explicitly.
type Repository interface {
	// Save persists an entry or reports it as skipped on composite-key collision.
	// DEDUPE: isNew=true indicates a new insert; isNew=false paired with err=nil
	// indicates a dedupe skip (the (Source, SourceID) pair was already present).
	// A non-nil err must be or wrap a *shared.HTTPError sentinel (today:
	// paper.ErrCatalogueUnavailable), so shared.AsHTTPError/errors.As can detect it.
	Save(ctx context.Context, e Entry) (isNew bool, err error)

	// FindByKey returns the stored entry or paper.ErrNotFound. On any other
	// storage failure, returns paper.ErrCatalogueUnavailable.
	FindByKey(ctx context.Context, source, sourceID string) (*Entry, error)

	// List returns every persisted entry, newest-first by SubmittedAt.
	// Empty result is a non-nil empty slice. On storage failure, returns
	// paper.ErrCatalogueUnavailable.
	List(ctx context.Context) ([]Entry, error)
}
