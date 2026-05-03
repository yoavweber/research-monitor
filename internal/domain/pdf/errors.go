package pdf

import "errors"

// ErrInvalidKey signals that a PDF key (or any value used to address a PDF
// artifact) failed validation — for example, an empty string, a malformed
// identifier, or a value that violates the key construction rules.
//
// Implementations and callers signal this category by wrapping the
// underlying cause with fmt.Errorf("...: %w", ErrInvalidKey, ...) and
// callers identify it via errors.Is(err, ErrInvalidKey).
var ErrInvalidKey = errors.New("pdf: invalid key")

// ErrFetch signals that retrieving PDF bytes from an upstream source
// failed — network errors, non-success HTTP responses, or any other
// upstream failure surfaced by a Fetcher implementation.
//
// Implementations wrap the underlying cause with %w and callers identify
// the category with errors.Is(err, ErrFetch).
var ErrFetch = errors.New("pdf: fetch failed")

// ErrStore signals that persisting or reading PDF bytes from the artifact
// store failed — object-store errors, I/O failures, or any other backend
// failure surfaced by a Store implementation.
//
// Implementations wrap the underlying cause with %w and callers identify
// the category with errors.Is(err, ErrStore).
var ErrStore = errors.New("pdf: store failed")
