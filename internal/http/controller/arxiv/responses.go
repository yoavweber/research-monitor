// Package arxiv hosts the HTTP handler and wire DTOs for the
// GET /api/arxiv/fetch endpoint. The domain layer (paper) carries no response
// shapes — this package owns the JSON contract and the mapping from
// []arxivapp.Result into it.
package arxiv

import (
	"time"

	arxivapp "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
)

// FetchEnvelope is the schema-only wrapper for the 200 response of
// GET /api/arxiv/fetch. It exists so the OpenAPI schema accurately describes
// the {"data": ...} runtime envelope; it is never instantiated.
type FetchEnvelope struct {
	Data FetchResponse `json:"data"`
}

// FetchResponse is the top-level wire shape for GET /api/arxiv/fetch. It is
// always wrapped by the common.Envelope "data" field at the controller layer.
//
// Job is the initial snapshot of the PDF-download job that was scheduled
// for the IsNew entries in this fetch. It is omitempty: when the fetch
// produced no new entries (or scheduling failed), the field is omitted
// entirely so the response shape is byte-identical to the pre-feature
// contract for empty fetches.
type FetchResponse struct {
	Entries   []EntryResponse                   `json:"entries"`
	Count     int                               `json:"count"`
	FetchedAt time.Time                         `json:"fetched_at"`
	Job       *paperctrl.DownloadJobSnapshotDTO `json:"job,omitempty"`
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

// ToFetchResponse maps the application FetchResult into the wire shape.
// Empty Entries marshals as "entries":[] (never null). Job is included
// only when scheduling produced a real job; a zero JobID elides the
// field via omitempty, keeping the pre-feature response byte-identical.
func ToFetchResponse(result arxivapp.FetchResult, fetchedAt time.Time) FetchResponse {
	resp := FetchResponse{
		Entries:   make([]EntryResponse, 0, len(result.Entries)),
		Count:     len(result.Entries),
		FetchedAt: fetchedAt,
	}
	for _, r := range result.Entries {
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
	if result.Job.JobID != pdfdownload.JobID("") {
		dto := paperctrl.ToDownloadJobSnapshotDTO(result.Job)
		resp.Job = &dto
	}
	return resp
}
