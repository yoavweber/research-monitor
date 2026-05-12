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
