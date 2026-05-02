package analyzer

import (
	"time"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
)

type SubmitAnalysisRequest struct {
	ExtractionID string `json:"extraction_id" binding:"required"`
}

type AnalysisResponse struct {
	ExtractionID         string    `json:"extraction_id"`
	ShortSummary         string    `json:"short_summary"`
	LongSummary          string    `json:"long_summary"`
	ThesisAngleFlag      bool      `json:"thesis_angle_flag"`
	ThesisAngleRationale string    `json:"thesis_angle_rationale"`
	Model                string    `json:"model"`
	PromptVersion        string    `json:"prompt_version"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AnalysisEnvelope struct {
	Data AnalysisResponse `json:"data"`
}

func ToAnalysisResponse(a domain.Analysis) AnalysisResponse {
	return AnalysisResponse{
		ExtractionID:         a.ExtractionID,
		ShortSummary:         a.ShortSummary,
		LongSummary:          a.LongSummary,
		ThesisAngleFlag:      a.ThesisAngleFlag,
		ThesisAngleRationale: a.ThesisAngleRationale,
		Model:                a.Model,
		PromptVersion:        a.PromptVersion,
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
	}
}
