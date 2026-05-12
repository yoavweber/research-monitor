// types.go: wire-and-state types for a PDF-download job. The Registry
// in service.go produces values of these types; the worker in worker.go
// mutates them under j.mu; HTTP controllers consume them through the
// reader interface defined in their own package. The download workflow
// is owned end-to-end by this package; paper.* is consulted only for
// paper identity.
package pdfdownload

import (
	"net/http"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Request is the minimal payload Schedule consumes. Carrying just
// (PaperID, PDFURL) keeps the input interface-segregated from the rest
// of paper.Entry.
type Request struct {
	PaperID paper.ID
	PDFURL  string
}

// RequestsFromEntries builds Requests from entries. Callers are
// responsible for filtering to the entries that should be downloaded
// (typically IsNew == true from Save). Returns a non-nil empty slice
// when entries is empty.
func RequestsFromEntries(entries []paper.Entry) []Request {
	out := make([]Request, 0, len(entries))
	for _, e := range entries {
		out = append(out, Request{
			PaperID: paper.IDFromEntry(e),
			PDFURL:  e.PDFURL,
		})
	}
	return out
}

// JobID identifies a PDF-download job within the active registry plus
// its retention window. UUIDv4 in production.
type JobID string

// EntryStatus is the lifecycle status of one entry in a download job.
// Values appear verbatim in SSE payloads and the status endpoint;
// renaming them is a breaking change for clients.
type EntryStatus string

const (
	StatusPending EntryStatus = "pending"
	StatusSuccess EntryStatus = "success"
	StatusFailed  EntryStatus = "failed"
)

// EntryResult is the per-entry outcome recorded in the registry and
// emitted on the SSE stream. On success Bytes is set; on failure
// Category (stable taxonomy mirroring pdf.Category*) and Description
// (sanitized detail) are set.
type EntryResult struct {
	PaperID     paper.ID
	Status      EntryStatus
	Bytes       int
	Category    string
	Description string
	CompletedAt time.Time
}

// JobSnapshot is the read-side projection of a download job.
type JobSnapshot struct {
	JobID       JobID
	Total       int
	Succeeded   int
	Failed      int
	Completed   bool
	CompletedAt time.Time
	Entries     []EntryResult
}

// Event is the sum type carried over Subscribe channels. Exactly one of
// Progress/Summary is non-nil per emission. Summary is emitted last and
// the channel closes immediately after.
type Event struct {
	JobID    JobID
	Progress *EntryResult
	Summary  *JobSnapshot
}

// ErrJobUnknown is returned by Snapshot/Subscribe when the job id has
// never been registered or has been evicted past its retention window.
// Carries a 404 so the HTTP ErrorEnvelope middleware renders it directly.
var ErrJobUnknown = shared.NewHTTPError(http.StatusNotFound, "download job unknown", nil)

// PDFArtifactKey is the source-scoped artifact identifier used as the
// SourceID of pdf.Key. For arXiv this concatenates SourceID and Version
// (e.g. "2404.12345v1"). Centralized here so the worker and any
// download-aware caller share the same cache-key rule.
func PDFArtifactKey(id paper.ID) string {
	return id.SourceID + id.Version
}
