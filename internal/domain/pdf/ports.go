package pdf

import (
	"context"
	"io"
)

// Store is the domain port for fetch-and-cache access to PDF artifacts. It
// hides whether the bytes are pulled from a local cache, fetched from an
// upstream URL and persisted, or served from any other backend: callers
// depend on this interface, never on the filesystem layout, the HTTP
// client, or the artifact-store SDK that ultimately satisfies the request.
//
// Implementations live under internal/infrastructure/pdf/ and are wired by
// bootstrap. The dependency-inversion intent is strict: domain and
// application code that needs a PDF on disk takes a Store, not a path or a
// URL, so swapping the backing store (local FS, S3, in-memory fake in
// tests) requires no change above this boundary.
type Store interface {
	// Ensure returns a Locator that addresses the bytes identified by key,
	// fetching and persisting them on the first call and serving from the
	// cache on subsequent calls. Implementations are expected to be
	// idempotent and safe under concurrent calls for the same key.
	//
	// Preconditions:
	//   - ctx must be non-nil. A cancelled ctx is honoured: the call returns
	//     promptly with a wrapped ctx.Err() and leaves no partial artifact
	//     under the canonical path.
	//   - key must satisfy Key.Validate. Implementations MUST call
	//     key.Validate() and surface a violation as an error wrapping
	//     ErrInvalidKey before performing any I/O.
	//
	// Postconditions on success (err == nil):
	//   - The returned Locator is non-nil.
	//   - Locator.Path() resolves to a complete, fully-written file
	//     containing the entire artifact bytes — never a partial or
	//     in-progress file.
	//   - Locator.Open(ctx) returns a reader over the same complete bytes.
	//
	// Postconditions on error (err != nil):
	//   - The returned Locator is unspecified and MUST NOT be used.
	//   - The error wraps exactly one of the following sentinels (callers
	//     identify the category via errors.Is):
	//       * ErrInvalidKey            — key failed Validate.
	//       * ErrFetch                 — upstream fetch failed (network
	//                                    error, non-success HTTP response,
	//                                    or any Fetcher-surfaced failure).
	//       * ErrStore                 — persisting or reading the artifact
	//                                    in the backing store failed.
	//       * a context.Context error  — ctx was cancelled or its deadline
	//                                    expired (errors.Is(err, ctx.Err())).
	//   - No partial file is left under the canonical path for key. If a
	//     write was started and aborted, the implementation cleans it up
	//     before returning.
	Ensure(ctx context.Context, key Key) (Locator, error)
}

// Locator is an opaque handle to a single PDF artifact that has already
// been materialized by a Store. It exposes two equivalent views over the
// same bytes — a real on-disk path for tools that only accept paths, and a
// stream for callers that prefer to read through io.Reader plumbing — so
// the rest of the system never has to choose a representation up front.
//
// A Locator is valid for the lifetime of the process that produced it.
// Implementations MUST NOT mutate, move, or delete the underlying file
// while any Locator referencing it is reachable.
type Locator interface {
	// Path returns a real, absolute on-disk path to the materialized PDF
	// file. The path is suitable for any path-only tool (for example,
	// `mineru -p <path>` or `pdftotext <path>`); it is not a virtual,
	// memory-backed, or URL-style locator.
	//
	// Stability invariants:
	//   - The returned string is stable for the entire lifetime of this
	//     Locator instance: repeated calls return the same value, and the
	//     file at that path continues to exist with identical contents
	//     until the Locator is no longer referenced.
	//   - The bytes at Path() agree byte-for-byte with the bytes produced
	//     by Open(ctx) on the same Locator instance.
	//
	// Path performs no I/O and never returns an error.
	Path() string

	// Open returns an io.ReadCloser positioned at the start of the same
	// bytes that Path() addresses. Callers MUST Close the returned reader;
	// failing to do so leaks the underlying file handle.
	//
	// Preconditions:
	//   - ctx must be non-nil. A cancelled ctx is honoured: implementations
	//     return promptly with a wrapped ctx.Err() and release any handle
	//     that was opened.
	//
	// Postconditions on success (err == nil):
	//   - The returned reader yields exactly the bytes of the artifact,
	//     byte-for-byte identical to those at Path().
	//
	// Postconditions on error (err != nil):
	//   - The returned reader is nil and there is nothing for the caller
	//     to close.
	//   - The error wraps either ErrStore (the file could not be opened
	//     because the backing store failed) or a context.Context error
	//     (errors.Is(err, ctx.Err())).
	Open(ctx context.Context) (io.ReadCloser, error)
}
