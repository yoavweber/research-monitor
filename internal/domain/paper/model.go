// Package paper is the source-neutral domain aggregate for academic papers
// ingested from external sources (arXiv today; other providers later).
// It owns the Entry value object, the Query value object, the Fetcher and
// Repository ports, and the error sentinels. It knows nothing about HTTP
// transport, XML/JSON wire formats, or any specific provider.
package paper

import "time"

// Entry is an immutable value object describing one paper fetched from a
// source. Fields are source-neutral: concrete adapters map provider-specific
// payloads into this shape. For the arXiv source, SourceID holds the arXiv
// identifier (e.g. "2404.12345") and Version holds the version suffix
// (e.g. "v1"). Entry carries no behavior.
type Entry struct {
	// Source identifies the upstream provider (e.g. "arxiv"). Required at persist time.
	Source string
	// SourceID is the provider's canonical identifier for the paper.
	SourceID string
	// Version is the provider's version suffix (e.g. "v1"); may be empty.
	Version         string
	Title           string
	Authors         []string
	Abstract        string
	PrimaryCategory string
	Categories      []string
	SubmittedAt     time.Time
	UpdatedAt       time.Time
	PDFURL          string
	AbsURL          string
}
