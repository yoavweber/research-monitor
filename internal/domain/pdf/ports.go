package pdf

import (
	"context"
	"io"
)

// Store fetches a PDF on first call for a given key and serves it from cache
// after that. Implementations live under internal/infrastructure/pdf/.
type Store interface {
	// Ensure returns a Locator for key's bytes. Idempotent and safe under
	// concurrent calls for the same key. On error no partial file is left
	// behind; callers identify the failure category with errors.Is against
	// the package's exported sentinels.
	Ensure(ctx context.Context, key Key) (Locator, error)
}

// Locator is a handle to a materialized PDF. It exposes both an on-disk path
// (for tools that take a path) and a stream. The file is stable for the
// lifetime of the process.
type Locator interface {
	// Path returns the absolute on-disk path. Stable across calls.
	Path() string

	// Open returns a reader over the same bytes. Caller must Close.
	Open(ctx context.Context) (io.ReadCloser, error)
}
