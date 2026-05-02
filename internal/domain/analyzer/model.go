// Package analyzer is the domain surface of the llm-analyzer feature: the
// persisted Analysis value type, the inbound UseCase / outbound Repository
// ports, and the sentinel errors that map onto HTTP via the shared
// HTTPError envelope. Composition with extraction.Repository and
// shared.LLMClient happens in application/analyzer.
package analyzer

import "time"

// Analysis is keyed by ExtractionID; CreatedAt is preserved across reruns
// and UpdatedAt advances on each.
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
