package paper

import (
	"fmt"
	"strings"
)

// idPathChars enumerates characters that must never appear in identity
// components used to construct artifact-store paths. Mirrors pdf.Key
// validation: the same forbidden characters that pdf.Key rejects, since
// (paper.ID).PDFArtifactKey feeds directly into pdf.Key.SourceID.
const idPathChars = `/\`

// ID is the stable identity of a paper across sources. (Source, SourceID)
// is unique within the catalogue; Version is part of identity for sources
// where each version is materialized as a distinct artifact (arXiv treats
// v1, v2, ... as distinct PDFs).
//
// ID is a value object: equality is structural, the zero value is invalid.
// Construction does not enforce validity; callers must invoke Validate
// before using an ID to address artifacts.
type ID struct {
	Source   string
	SourceID string
	Version  string
}

// NewID is the direct constructor.
func NewID(source, sourceID, version string) ID {
	return ID{Source: source, SourceID: sourceID, Version: version}
}

// IDFromEntry is the convenience constructor when an Entry is in scope.
// Lives as a free function rather than a method on Entry to preserve the
// existing "Entry carries no behavior" convention documented in model.go.
func IDFromEntry(e Entry) ID {
	return ID{Source: e.Source, SourceID: e.SourceID, Version: e.Version}
}

// Validate returns nil if the ID is well-formed, or an error wrapping
// ErrInvalidID describing the first violation it finds. Callers
// discriminate via errors.Is(err, ErrInvalidID).
func (i ID) Validate() error {
	if strings.TrimSpace(i.Source) == "" {
		return fmt.Errorf("source must not be empty: %w", ErrInvalidID)
	}
	if strings.TrimSpace(i.SourceID) == "" {
		return fmt.Errorf("source id must not be empty: %w", ErrInvalidID)
	}
	if strings.Contains(i.Source, "..") || strings.ContainsAny(i.Source, idPathChars) {
		return fmt.Errorf("source must not contain path separator or traversal characters: %w", ErrInvalidID)
	}
	if strings.Contains(i.SourceID, "..") || strings.ContainsAny(i.SourceID, idPathChars) {
		return fmt.Errorf("source id must not contain path separator or traversal characters: %w", ErrInvalidID)
	}
	if strings.Contains(i.Version, "..") || strings.ContainsAny(i.Version, idPathChars) {
		return fmt.Errorf("version must not contain path separator or traversal characters: %w", ErrInvalidID)
	}
	return nil
}

// PDFArtifactKey returns the source-scoped artifact identifier used as
// the SourceID of pdf.Key. For arXiv this concatenates SourceID and
// Version (e.g. "2404.12345v1"). Centralizing the rule here prevents
// cache-key drift between the worker and any other call site that
// builds a pdf.Key.
func (i ID) PDFArtifactKey() string {
	return i.SourceID + i.Version
}
