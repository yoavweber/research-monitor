package paper

// ID is the stable identity of a paper across sources. (Source, SourceID)
// is unique within the catalogue; Version is part of identity for sources
// where each version is a distinct artifact (arXiv treats v1, v2, ... as
// distinct PDFs).
type ID struct {
	Source   string
	SourceID string
	Version  string
}

// IDFromEntry copies the identity triple out of an Entry. Free function
// rather than a method to preserve Entry's "no behavior" convention.
func IDFromEntry(e Entry) ID {
	return ID{Source: e.Source, SourceID: e.SourceID, Version: e.Version}
}

// PDFArtifactKey returns the source-scoped artifact identifier used as
// the SourceID of pdf.Key. For arXiv this concatenates SourceID and
// Version (e.g. "2404.12345v1"). Centralized here so the worker and any
// future caller share the same cache-key rule.
func (i ID) PDFArtifactKey() string {
	return i.SourceID + i.Version
}
