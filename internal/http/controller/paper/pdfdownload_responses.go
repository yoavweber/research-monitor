package paper

import (
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// PaperIDDTO is the wire shape for paper.ID. Field names are the canonical
// snake_case contract; the same shape is reused inside download status
// responses and SSE event payloads. Version is omitted from the JSON when
// the source has no version concept (omitempty preserves backward-compat
// for non-versioned sources).
type PaperIDDTO struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id"`
	Version  string `json:"version,omitempty"`
}

// DownloadEntryResultDTO mirrors paper.DownloadEntryResult for the wire.
// On success Bytes is populated; on failure Category and Description are
// populated; on Pending only Status is set (other fields are zero values
// and either omit via omitempty or marshal as zero).
type DownloadEntryResultDTO struct {
	PaperID     PaperIDDTO `json:"paper_id"`
	Status      string     `json:"status"`
	Bytes       int        `json:"bytes,omitempty"`
	Category    string     `json:"category,omitempty"`
	Description string     `json:"description,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// DownloadJobSnapshotDTO is the wire shape for paper.DownloadJobSnapshot.
// Used by both the GET /downloads/{job_id} status endpoint (wrapped in the
// JobStatusEnvelope) and embedded as the Job field of the arxiv fetch
// response.
type DownloadJobSnapshotDTO struct {
	JobID       string                   `json:"job_id"`
	Total       int                      `json:"total"`
	Succeeded   int                      `json:"succeeded"`
	Failed      int                      `json:"failed"`
	Completed   bool                     `json:"completed"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
	Entries     []DownloadEntryResultDTO `json:"entries"`
}

// JobStatusEnvelope is the schema-only wrapper for the 200 response of
// GET /api/arxiv/downloads/{job_id}. Mirrors the FetchEnvelope pattern;
// never instantiated, exists for the OpenAPI schema.
type JobStatusEnvelope struct {
	Data DownloadJobSnapshotDTO `json:"data"`
}

// DownloadProgressEventDTO is the data payload of a `download.progress`
// SSE frame. It is a thin alias of DownloadEntryResultDTO; defined as a
// distinct type so the SSE wire contract stays explicit.
type DownloadProgressEventDTO = DownloadEntryResultDTO

// DownloadSummaryEventDTO is the data payload of the terminal
// `download.summary` SSE frame. It carries only the totals so a client
// that has been streaming progress events does not need to redownload
// per-entry details.
type DownloadSummaryEventDTO struct {
	JobID     string `json:"job_id"`
	Total     int    `json:"total"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
}

// ToPaperIDDTO maps a paper.ID into its wire shape.
func ToPaperIDDTO(id paper.ID) PaperIDDTO {
	return PaperIDDTO{
		Source:   id.Source,
		SourceID: id.SourceID,
		Version:  id.Version,
	}
}

// ToDownloadEntryResultDTO maps a per-entry result into its wire shape.
// Pending results carry a zero CompletedAt; this is rendered as an
// omitted field via the *time.Time pointer.
func ToDownloadEntryResultDTO(r paper.DownloadEntryResult) DownloadEntryResultDTO {
	dto := DownloadEntryResultDTO{
		PaperID:     ToPaperIDDTO(r.PaperID),
		Status:      string(r.Status),
		Bytes:       r.Bytes,
		Category:    r.Category,
		Description: r.Description,
	}
	if !r.CompletedAt.IsZero() {
		ct := r.CompletedAt
		dto.CompletedAt = &ct
	}
	return dto
}

// ToDownloadJobSnapshotDTO maps the domain snapshot into its wire shape.
// A nil or empty Entries slice maps to a non-nil zero-length slice so the
// JSON renders as "entries":[] rather than "entries":null.
func ToDownloadJobSnapshotDTO(s paper.DownloadJobSnapshot) DownloadJobSnapshotDTO {
	entries := make([]DownloadEntryResultDTO, 0, len(s.Entries))
	for _, e := range s.Entries {
		entries = append(entries, ToDownloadEntryResultDTO(e))
	}
	dto := DownloadJobSnapshotDTO{
		JobID:     string(s.JobID),
		Total:     s.Total,
		Succeeded: s.Succeeded,
		Failed:    s.Failed,
		Completed: s.Completed,
		Entries:   entries,
	}
	if !s.CompletedAt.IsZero() {
		ct := s.CompletedAt
		dto.CompletedAt = &ct
	}
	return dto
}
