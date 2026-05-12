package paper

import "time"

// PDFDownloadRequest is the minimal payload the PDFScheduler port
// consumes. Carrying just (PaperID, PDFURL) keeps the port
// interface-segregated from the rest of paper.Entry.
type PDFDownloadRequest struct {
	PaperID ID
	PDFURL  string
}

// NewPDFDownloadRequests builds requests from entries. Callers are
// responsible for filtering to the entries that should be downloaded
// (typically IsNew == true from Save). Returns a non-nil empty slice
// when entries is empty.
func NewPDFDownloadRequests(entries []Entry) []PDFDownloadRequest {
	out := make([]PDFDownloadRequest, 0, len(entries))
	for _, e := range entries {
		out = append(out, PDFDownloadRequest{
			PaperID: IDFromEntry(e),
			PDFURL:  e.PDFURL,
		})
	}
	return out
}

// DownloadJobID identifies a PDF-download job within the active
// registry plus its retention window. UUIDv4 in production.
type DownloadJobID string

// DownloadEntryStatus is the lifecycle status of one entry in a
// download job. Values appear verbatim in SSE payloads and the status
// endpoint; renaming them is a breaking change for clients.
type DownloadEntryStatus string

const (
	DownloadStatusPending DownloadEntryStatus = "pending"
	DownloadStatusSuccess DownloadEntryStatus = "success"
	DownloadStatusFailed  DownloadEntryStatus = "failed"
)

// DownloadEntryResult is the per-entry outcome recorded in the registry
// and emitted on the SSE stream. On success Bytes is set; on failure
// Category (stable taxonomy mirroring pdf.Category*) and Description
// (sanitized detail) are set.
type DownloadEntryResult struct {
	PaperID     ID
	Status      DownloadEntryStatus
	Bytes       int
	Category    string
	Description string
	CompletedAt time.Time
}

// DownloadJobSnapshot is the read-side projection of a download job.
type DownloadJobSnapshot struct {
	JobID       DownloadJobID
	Total       int
	Succeeded   int
	Failed      int
	Completed   bool
	CompletedAt time.Time
	Entries     []DownloadEntryResult
}

// DownloadEvent is the sum type carried over Subscribe channels.
// Exactly one of Progress/Summary is non-nil per emission. Summary is
// emitted last and the channel closes immediately after.
type DownloadEvent struct {
	JobID    DownloadJobID
	Progress *DownloadEntryResult
	Summary  *DownloadJobSnapshot
}
