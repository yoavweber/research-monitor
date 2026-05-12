package paper

import (
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// PaperIDDTO is the wire shape for paper.ID.
type PaperIDDTO struct {
	Source   string `json:"source"`
	SourceID string `json:"source_id"`
	Version  string `json:"version,omitempty"`
}

// DownloadEntryResultDTO is the wire shape for one download outcome.
type DownloadEntryResultDTO struct {
	PaperID     PaperIDDTO `json:"paper_id"`
	Status      string     `json:"status"`
	Bytes       int        `json:"bytes,omitempty"`
	Category    string     `json:"category,omitempty"`
	Description string     `json:"description,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// DownloadJobSnapshotDTO is the wire shape for a download-job snapshot.
type DownloadJobSnapshotDTO struct {
	JobID       string                   `json:"job_id"`
	Total       int                      `json:"total"`
	Succeeded   int                      `json:"succeeded"`
	Failed      int                      `json:"failed"`
	Completed   bool                     `json:"completed"`
	CompletedAt *time.Time               `json:"completed_at,omitempty"`
	Entries     []DownloadEntryResultDTO `json:"entries"`
}

// JobStatusEnvelope wraps the 200 response of GET /downloads/{job_id}
// for the OpenAPI schema.
type JobStatusEnvelope struct {
	Data DownloadJobSnapshotDTO `json:"data"`
}

// DownloadProgressEventDTO is the data payload of a `download.progress` SSE frame.
type DownloadProgressEventDTO = DownloadEntryResultDTO

// DownloadSummaryEventDTO is the data payload of the terminal `download.summary` SSE frame.
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
func ToDownloadEntryResultDTO(r pdfdownload.EntryResult) DownloadEntryResultDTO {
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

// ToDownloadJobSnapshotDTO maps the snapshot into its wire shape. A nil
// or empty Entries slice maps to a non-nil zero-length slice so the
// JSON renders as "entries":[] rather than "entries":null.
func ToDownloadJobSnapshotDTO(s pdfdownload.JobSnapshot) DownloadJobSnapshotDTO {
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
