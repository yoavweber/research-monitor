package paper

import (
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// PaperResponse is the per-paper wire shape. Field names are the canonical
// snake_case contract; downstream consumers key off these. Keep them stable.
type PaperResponse struct {
	Source          string    `json:"source"`
	SourceID        string    `json:"source_id"`
	Version         string    `json:"version,omitempty"`
	Title           string    `json:"title"`
	Authors         []string  `json:"authors"`
	Abstract        string    `json:"abstract"`
	PrimaryCategory string    `json:"primary_category"`
	Categories      []string  `json:"categories"`
	SubmittedAt     time.Time `json:"submitted_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	PDFURL          string    `json:"pdf_url"`
	AbsURL          string    `json:"abs_url"`
}

// PaperListResponse is the top-level wire shape for GET /api/papers. It is
// always wrapped by the common.Envelope "data" field at the controller layer.
type PaperListResponse struct {
	Papers []PaperResponse `json:"papers"`
	Count  int             `json:"count"`
}

// ToPaperResponse maps a single domain Entry into its wire DTO.
func ToPaperResponse(e paper.Entry) PaperResponse {
	return PaperResponse{
		Source:          e.Source,
		SourceID:        e.SourceID,
		Version:         e.Version,
		Title:           e.Title,
		Authors:         e.Authors,
		Abstract:        e.Abstract,
		PrimaryCategory: e.PrimaryCategory,
		Categories:      e.Categories,
		SubmittedAt:     e.SubmittedAt,
		UpdatedAt:       e.UpdatedAt,
		PDFURL:          e.PDFURL,
		AbsURL:          e.AbsURL,
	}
}

// ToPaperListResponse maps a slice of domain Entry values into the list wire
// shape. A nil or empty slice yields a non-nil, zero-length Papers so JSON
// marshals to "papers":[] (not "papers":null) — this is the empty-catalogue
// contract; clients distinguish "no papers" from "absent field" off this.
func ToPaperListResponse(entries []paper.Entry) PaperListResponse {
	resp := PaperListResponse{
		Papers: make([]PaperResponse, 0, len(entries)),
		Count:  len(entries),
	}
	for _, e := range entries {
		resp.Papers = append(resp.Papers, ToPaperResponse(e))
	}
	return resp
}
