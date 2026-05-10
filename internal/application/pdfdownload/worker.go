package pdfdownload

import (
	"os"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

// runJob drives a single download job to completion. It is launched as a
// goroutine from SchedulePDFDownloads and runs entirely on the registry's
// background context — never the caller's ctx (R3.5). A failure on any
// entry is classified and recorded, but the loop continues so a single
// bad URL cannot starve the rest of the batch (R3.3).
//
// Tasks 2.3+ extend this function with subscriber fan-out and a final
// Summary event. For 2.2 the post-loop step is limited to flipping the
// completion flag so tests can synchronise on the worker finishing.
func (r *Registry) runJob(j *job) {
	defer r.workers.Done()

	for i, req := range j.requests {
		result := r.runEntry(j.id, req)
		// Overwrite the pre-seeded Pending entry at the same index so
		// the entries slice mirrors j.requests by position. Future
		// tasks rely on this ordering for the SSE backlog projection.
		j.mu.Lock()
		j.entries[i] = result
		j.mu.Unlock()
	}

	j.mu.Lock()
	j.completed = true
	j.mu.Unlock()
}

// runEntry is the per-entry slice of the worker contract. It builds the
// pdf.Key from the request, calls Store.Ensure on the registry-owned
// background context, classifies the outcome, and emits the structured
// log used by dashboards. The retry/backoff seam is marked in-line above
// the Ensure call (Non-Goal in v1 per design.md).
func (r *Registry) runEntry(jobID paper.DownloadJobID, req paper.PDFDownloadRequest) paper.DownloadEntryResult {
	key := pdf.Key{
		SourceType: req.PaperID.Source,
		SourceID:   req.PaperID.PDFArtifactKey(),
		URL:        req.PDFURL,
	}

	// TODO: future retry/backoff seam
	loc, err := r.store.Ensure(r.bgCtx, key)

	status, category, description := classify(err)
	result := paper.DownloadEntryResult{
		PaperID:     req.PaperID,
		Status:      status,
		Category:    category,
		Description: description,
		CompletedAt: r.clock.Now(),
	}
	if err == nil && loc != nil {
		// The store guarantees a materialised file on success; reading
		// its size via os.Stat avoids re-opening the bytes. The error
		// path is treated as a zero-byte success: the file exists, we
		// just could not measure it. That keeps Status=success honest
		// (R3.2) without inventing a new failure mode.
		if fi, statErr := os.Stat(loc.Path()); statErr == nil {
			result.Bytes = int(fi.Size())
		}
	}

	r.emitEntryCompleted(jobID, result)
	return result
}

// emitEntryCompleted writes the structured log for one finished entry.
// Info on success, Warn on failure. Field shape mirrors design.md so
// dashboards and tests share a single source of truth.
func (r *Registry) emitEntryCompleted(jobID paper.DownloadJobID, result paper.DownloadEntryResult) {
	args := []any{
		"job_id", string(jobID),
		"paper_id", formatPaperID(result.PaperID),
		"status", string(result.Status),
	}
	if result.Status == paper.DownloadStatusSuccess {
		args = append(args, "bytes", result.Bytes)
		r.logger.InfoContext(r.bgCtx, "pdfdownload.entry.completed", args...)
		return
	}
	args = append(args, "category", result.Category, "description", result.Description)
	r.logger.WarnContext(r.bgCtx, "pdfdownload.entry.completed", args...)
}
