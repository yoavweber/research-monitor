package pdf

import (
	"fmt"
	"strings"
)

// pathChars enumerates characters that must never appear in identifier
// fields used to construct artifact-store paths. Defense-in-depth against
// path traversal: even though upstream callers are expected to sanitize
// input, the domain rejects these characters at the boundary.
const pathChars = `/\`

// Key is the value object that uniquely identifies a PDF artifact in the
// store. It carries the upstream source classification, the source-scoped
// identifier, and the URL from which the bytes were (or will be) fetched.
//
// A Key is valid iff every field is non-empty after whitespace trimming
// and the identifier fields contain no path-separator or path-traversal
// characters. Construction does not enforce validity; callers must invoke
// Validate before using a Key to address artifacts.
type Key struct {
	// SourceType classifies the upstream origin (e.g. "paper").
	//
	// Must be non-empty after trimming and must not contain "..", "/",
	// or "\" — these characters are forbidden because SourceType is used
	// as a path segment when addressing artifacts in the store, and
	// allowing them would enable path traversal.
	SourceType string

	// SourceID is the source-scoped identifier (e.g. "2404.12345v1").
	//
	// Must be non-empty after trimming and must not contain "..", "/",
	// or "\" — these characters are forbidden because SourceID is used
	// as a path segment when addressing artifacts in the store, and
	// allowing them would enable path traversal.
	SourceID string

	// URL is the upstream PDF URL from which bytes are fetched.
	//
	// Must be non-empty after trimming. Path-separator characters are
	// permitted here because URLs legitimately contain them; URL is not
	// used to construct artifact-store paths.
	URL string
}

// Validate returns nil if the Key is well-formed, or an error wrapping
// ErrInvalidKey describing the first violation it finds. Callers
// identify the rejection category via errors.Is(err, ErrInvalidKey).
func (k Key) Validate() error {
	if strings.TrimSpace(k.SourceType) == "" {
		return fmt.Errorf("source type must not be empty: %w", ErrInvalidKey)
	}
	if strings.TrimSpace(k.SourceID) == "" {
		return fmt.Errorf("source id must not be empty: %w", ErrInvalidKey)
	}
	if strings.TrimSpace(k.URL) == "" {
		return fmt.Errorf("url must not be empty: %w", ErrInvalidKey)
	}

	if strings.Contains(k.SourceType, "..") || strings.ContainsAny(k.SourceType, pathChars) {
		return fmt.Errorf("source type must not contain path separator or traversal characters: %w", ErrInvalidKey)
	}
	if strings.Contains(k.SourceID, "..") || strings.ContainsAny(k.SourceID, pathChars) {
		return fmt.Errorf("source id must not contain path separator or traversal characters: %w", ErrInvalidKey)
	}

	return nil
}
