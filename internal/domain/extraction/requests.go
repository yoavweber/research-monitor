package extraction

import "strings"

// SubmitRequest is the inbound DTO for POST /api/extractions, prior to
// transport-level concerns. v1 only accepts source_type == "paper".
type SubmitRequest struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	PDFPath    string `json:"pdf_path"`
}

// Validate enforces the per-field invariants the controller and use case both
// rely on: each field is non-empty after trimming, and source_type is exactly
// "paper". Empty/whitespace fields produce ErrInvalidRequest; any other
// source_type produces ErrUnsupportedSourceType. Empty fields are checked
// before the source_type value so an entirely missing payload reports the
// shape error first.
func (r SubmitRequest) Validate() error {
	if strings.TrimSpace(r.SourceType) == "" ||
		strings.TrimSpace(r.SourceID) == "" ||
		strings.TrimSpace(r.PDFPath) == "" {
		return ErrInvalidRequest
	}
	if r.SourceType != "paper" {
		return ErrUnsupportedSourceType
	}
	return nil
}
