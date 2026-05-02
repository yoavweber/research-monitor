package analyzer

import (
	"encoding/json"
	"strings"
)

// Pointer fields distinguish "field absent" from "field present with zero
// value" — required so missing flag and missing rationale both register as
// malformed.
type thesisEnvelope struct {
	Flag      *bool   `json:"flag"`
	Rationale *string `json:"rationale"`
}

type parsedThesis struct {
	Flag      bool
	Rationale string
}

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
