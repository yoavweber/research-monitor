// Package arxiv hosts the HTTP handler and wire DTOs for the
// GET /api/arxiv/fetch endpoint. The domain layer (paper) carries no response
// shapes — this package owns the JSON contract and the mapping from
// []paper.Entry into it.
package arxiv

import (
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// FetchResponse is the top-level wire shape for GET /api/arxiv/fetch. It is
// always wrapped by the common.Envelope "data" field at the controller layer.
type FetchResponse struct {
	Entries   []EntryResponse `json:"entries"`
	Count     int             `json:"count"`
	FetchedAt time.Time       `json:"fetched_at"`
}

// EntryResponse is the per-paper wire shape. Field names are the canonical
// snake_case contract; a future consumer (dedupe / summarisation pipeline) keys
// off these. Keep them stable.
type EntryResponse struct {
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

// ToFetchResponse maps domain Entry values into the FetchResponse wire shape.
// A nil or empty slice yields a non-nil, zero-length Entries so JSON marshals
// to "entries":[] (not "entries":null) — required by requirement 1.5.
func ToFetchResponse(entries []paper.Entry, fetchedAt time.Time) FetchResponse {
	resp := FetchResponse{
		Entries:   make([]EntryResponse, 0, len(entries)),
		Count:     len(entries),
		FetchedAt: fetchedAt,
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, EntryResponse{
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
		})
	}
	return resp
}
