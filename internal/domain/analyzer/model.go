// Package analyzer is the domain surface of the llm-analyzer feature: the
// persisted Analysis value type, the inbound UseCase / outbound Repository
// ports, and the sentinel errors that map onto HTTP via the shared
// HTTPError envelope. Composition with extraction.Repository and
// shared.LLMClient happens in application/analyzer.
package analyzer

import "time"

// Analysis is keyed by ExtractionID; CreatedAt is preserved across reruns
// and UpdatedAt advances on each.
//
// ThesisAngleFlag and ThesisAngleRationale are placeholder columns: this
// slice does not run a thesis classifier, the use case fills them with a
// constant default. They will become real once the thesis-profile follow-up
// spec defines a classification criterion. The columns ship now so the wire
// shape doesn't churn when the classifier lands.
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
