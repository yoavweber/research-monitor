package analyzer

import (
	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

// SubmitAnalysisRequest is the wire shape of POST /analyses. ExtractionID is
// the only field; binding rejects empty strings via the binding:"required"
// tag and bound errors are surfaced via the standard 400 envelope.
type SubmitAnalysisRequest struct {
	ExtractionID string `json:"extraction_id" binding:"required"`
}

// AnalysisResponse is the wire shape returned by POST /analyses (200) and
// GET /analyses/:extraction_id (200). All fields are always present;
// rationale and summaries may be empty strings if the model produced them.
type AnalysisResponse struct {
	ExtractionID         string `json:"extraction_id"`
	ShortSummary         string `json:"short_summary"`
	LongSummary          string `json:"long_summary"`
	ThesisAngleFlag      bool   `json:"thesis_angle_flag"`
	ThesisAngleRationale string `json:"thesis_angle_rationale"`
	Model                string `json:"model"`
	PromptVersion        string `json:"prompt_version"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

// AnalysisEnvelope is the @Success-annotation wrapper used by swag. Mirrors
// the runtime envelope produced by common.Data.
type AnalysisEnvelope struct {
	Data AnalysisResponse `json:"data"`
}

// ToAnalysisResponse maps a domain.Analysis to the wire DTO. Timestamps are
// RFC3339 strings so the wire shape is stable across drivers and locales.
func ToAnalysisResponse(a domain.Analysis) AnalysisResponse {
	return AnalysisResponse{
		ExtractionID:         a.ExtractionID,
		ShortSummary:         a.ShortSummary,
		LongSummary:          a.LongSummary,
		ThesisAngleFlag:      a.ThesisAngleFlag,
		ThesisAngleRationale: a.ThesisAngleRationale,
		Model:                a.Model,
		PromptVersion:        a.PromptVersion,
		CreatedAt:            a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:            a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
