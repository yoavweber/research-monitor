package paper

import (
	"fmt"
	"strings"
	"time"
)

// PDFDownloadRequest is the minimal payload the PDFScheduler port consumes.
// Carrying just (PaperID, PDFURL) keeps the port interface-segregated from
// the rest of paper.Entry: the scheduler implementation never depends on
// title, abstract, authors, or any other Entry field.
type PDFDownloadRequest struct {
	PaperID ID
	PDFURL  string
}

// Validate returns nil if the request is well-formed, or an error wrapping
// ErrInvalidPDFDownloadRequest. An invalid PaperID is surfaced verbatim
// (errors.Is matches ErrInvalidID); other failures wrap
// ErrInvalidPDFDownloadRequest.
func (r PDFDownloadRequest) Validate() error {
	if err := r.PaperID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.PDFURL) == "" {
		return fmt.Errorf("pdf url must not be empty: %w", ErrInvalidPDFDownloadRequest)
	}
	return nil
}

// NewPDFDownloadRequests builds requests from entries. The caller is
// responsible for filtering to only the entries that should be downloaded
// (e.g. those whose persistence Save returned IsNew == true) before
// calling. Returns a non-nil empty slice when entries is empty.
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

// DownloadJobID identifies a PDF-download job within the active registry
// plus its retention window. UUIDv4 in production; tests may use any
// stable string.
type DownloadJobID string

// DownloadEntryStatus is the lifecycle status of one entry in a download
// job. The wire-contract values appear in SSE payloads and the status
// endpoint; renaming them is a breaking change for clients.
type DownloadEntryStatus string

const (
	DownloadStatusPending DownloadEntryStatus = "pending"
	DownloadStatusSuccess DownloadEntryStatus = "success"
	DownloadStatusFailed  DownloadEntryStatus = "failed"
)

// DownloadEntryResult is the per-entry outcome recorded in the registry
// and emitted on the SSE stream. On success Bytes is populated; on
// failure Category (a stable taxonomy mirroring pdf.Category*) and
// Description (sanitized human-readable detail) are populated. On the
// initial Pending state both Status and CompletedAt are zero values
// other than Status itself.
type DownloadEntryResult struct {
	PaperID     ID
	Status      DownloadEntryStatus
	Bytes       int
	Category    string
	Description string
	CompletedAt time.Time
}

// DownloadJobSnapshot is the read-side projection of a download job:
// totals, completion flag, and the per-entry results. Returned by the
// PDFDownloadReader.SnapshotPDFDownloadJob port and embedded in the
// arxiv fetch response and the SSE summary frame.
type DownloadJobSnapshot struct {
	JobID       DownloadJobID
	Total       int
	Succeeded   int
	Failed      int
	Completed   bool
	CompletedAt time.Time
	Entries     []DownloadEntryResult
}

// DownloadEvent is the sum type carried over PDFDownloadReader.Subscribe
// channels. Exactly one of Progress/Summary is non-nil per emission.
// Summary is emitted last; the channel closes immediately after.
type DownloadEvent struct {
	JobID    DownloadJobID
	Progress *DownloadEntryResult
	Summary  *DownloadJobSnapshot
}
