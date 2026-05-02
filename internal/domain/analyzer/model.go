// Package analyzer is the domain surface of the llm-analyzer feature: the
// persisted Analysis value type, the inbound UseCase / outbound Repository
// ports, and the sentinel errors that map onto HTTP status codes via the
// shared HTTPError envelope. Domain code holds no infrastructure or
// transport details; composition with extraction.Repository and
// shared.LLMClient happens in application/analyzer.
package analyzer

import "time"

// Analysis is the persisted result of one LLM analysis run for a given
// extraction. Identity is ExtractionID; the upsert contract guarantees at
// most one Analysis per ExtractionID at any time, with CreatedAt preserved
// across reruns and UpdatedAt advancing on each.
type Analysis struct {
	ExtractionID         string
	ShortSummary         string
	LongSummary          string
	ThesisAngleFlag      bool
	ThesisAngleRationale string
	Model                string
	PromptVersion        string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
