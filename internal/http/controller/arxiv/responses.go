// Package arxiv hosts the HTTP handler and wire DTOs for the
// GET /api/arxiv/fetch endpoint. The domain layer (paper) carries no response
// shapes — this package owns the JSON contract and the mapping from
// []arxivapp.Result into it.
package arxiv

import (
	"time"

	arxivapp "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
)

// FetchEnvelope is the schema-only wrapper for the 200 response of
// GET /api/arxiv/fetch. It exists so the OpenAPI schema accurately describes
// the {"data": ...} runtime envelope; it is never instantiated.
type FetchEnvelope struct {
	Data FetchResponse `json:"data"`
}

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
	IsNew           bool      `json:"is_new"`
}

// ToFetchResponse maps the fetched results into the FetchResponse wire shape.
// A nil or empty slice yields a non-nil, zero-length Entries so JSON marshals
// to "entries":[] (not "entries":null) — required by requirement 1.5. is_new
// is propagated from the application layer's per-entry persist result (R5.3).
func ToFetchResponse(results []arxivapp.Result, fetchedAt time.Time) FetchResponse {
	resp := FetchResponse{
		Entries:   make([]EntryResponse, 0, len(results)),
		Count:     len(results),
		FetchedAt: fetchedAt,
	}
	for _, r := range results {
		e := r.Entry
		resp.Entries = append(resp.Entries, EntryResponse{
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
			IsNew:           r.IsNew,
		})
	}
	return resp
}
