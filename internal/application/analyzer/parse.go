package analyzer

import (
	"encoding/json"
	"strings"
)

// thesisEnvelope is the typed shape the thesis-angle LLM response must
// satisfy. The pointer fields let the parser distinguish "field absent" from
// "field present with zero value" — required for Requirement 6.3 (missing
// flag and missing rationale must both be treated as malformed).
type thesisEnvelope struct {
	Flag      *bool   `json:"flag"`
	Rationale *string `json:"rationale"`
}

// parsedThesis is the normalized output the use case persists.
type parsedThesis struct {
	Flag      bool
	Rationale string
}

// parseThesisEnvelope decodes the LLM's thesis-angle response into the
// validated envelope shape. It returns (parsedThesis, true) on success and
// the zero parsedThesis with false on any of the malformed conditions:
// non-JSON body, missing required fields, wrong field types, or empty
// rationale. Extra fields are tolerated. Surrounding whitespace is stripped.
func parseThesisEnvelope(raw string) (parsedThesis, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return parsedThesis{}, false
	}

	var env thesisEnvelope
	if err := json.Unmarshal([]byte(trimmed), &env); err != nil {
		return parsedThesis{}, false
	}
	if env.Flag == nil || env.Rationale == nil {
		return parsedThesis{}, false
	}
	rationale := strings.TrimSpace(*env.Rationale)
	if rationale == "" {
		return parsedThesis{}, false
	}
	return parsedThesis{Flag: *env.Flag, Rationale: rationale}, true
}
