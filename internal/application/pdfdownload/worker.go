package pdfdownload

import (
	"os"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

// runJob drives a single download job to completion on the registry's
// background context. Per-entry failures are classified and recorded;
// the loop continues so a single bad URL cannot starve the batch.
// Logging happens outside j.mu to keep the critical section tight.
func (r *Registry) runJob(j *job) {
	defer r.workers.Done()

	startedAt := r.clock.Now()
	j.mu.Lock()
	j.startedAt = startedAt
	j.mu.Unlock()

	for i, req := range j.requests {
		result := r.runEntry(req)
		ev := paper.DownloadEvent{JobID: j.id, Progress: &result}
		dropped := r.appendAndFanOut(j, i, result, ev)
		r.emitEntryCompleted(j.id, result)
		r.emitSubscriberDropped(j.id, dropped)
	}

	completedAt := r.clock.Now()
	dropped, summary := r.finishJob(j, completedAt)
	r.emitSubscriberDropped(j.id, dropped)

	duration := completedAt.Sub(startedAt)
	r.logger.InfoContext(r.bgCtx, "pdfdownload.job.completed",
		"job_id", string(j.id),
		"total", summary.Total,
		"succeeded", summary.Succeeded,
		"failed", summary.Failed,
		"duration_ms", duration.Milliseconds(),
	)
}

func (r *Registry) emitSubscriberDropped(jobID paper.DownloadJobID, count int) {
	for range count {
		r.logger.WarnContext(r.bgCtx, "pdfdownload.subscriber.dropped",
			"job_id", string(jobID),
			"reason", "slow_consumer",
		)
	}
}

// appendAndFanOut commits one entry result, appends its Progress event,
// and fans out to subscribers — all atomically under j.mu so a
// subscriber attached mid-job sees the event via exactly one of backlog
// (from Subscribe) or live channel (from fan-out), never both, never
// neither. Returns the number of subscribers dropped this call.
func (r *Registry) appendAndFanOut(j *job, idx int, result paper.DownloadEntryResult, ev paper.DownloadEvent) int {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.entries[idx] = result
	j.events = append(j.events, ev)
	return fanOutLocked(j, ev)
}

// finishJob emits the terminal Summary event, closes remaining
// subscribers, and flips completed/completedAt — all under j.mu so no
// event can arrive after Summary on any subscriber.
func (r *Registry) finishJob(j *job, completedAt time.Time) (dropped int, snapshot paper.DownloadJobSnapshot) {
	j.mu.Lock()
	defer j.mu.Unlock()

	snap := buildSnapshotLocked(j, true, completedAt)
	ev := paper.DownloadEvent{JobID: j.id, Summary: &snap}
	j.events = append(j.events, ev)
	dropped = fanOutLocked(j, ev)

	for _, ch := range j.subscribers {
		close(ch)
	}
	j.subscribers = nil

	j.completed = true
	j.completedAt = completedAt

	return dropped, snap
}

// fanOutLocked uses a non-blocking send so a stalled consumer never
// blocks the worker; overfilled subscribers are closed and removed.
// Caller must hold j.mu.
func fanOutLocked(j *job, ev paper.DownloadEvent) int {
	if len(j.subscribers) == 0 {
		return 0
	}
	kept := j.subscribers[:0]
	dropped := 0
	for _, ch := range j.subscribers {
		select {
		case ch <- ev:
			kept = append(kept, ch)
		default:
			close(ch)
			dropped++
		}
	}
	// Zero the tail so dropped channels are not retained by the array.
	for i := len(kept); i < len(j.subscribers); i++ {
		j.subscribers[i] = nil
	}
	j.subscribers = kept
	return dropped
}

// buildSnapshotLocked projects j into a DownloadJobSnapshot. Caller
// must hold j.mu. The entries slice is copied so the result is safe to
// hand outside the critical section.
func buildSnapshotLocked(j *job, completed bool, completedAt time.Time) paper.DownloadJobSnapshot {
	var succeeded, failed int
	for _, e := range j.entries {
		switch e.Status {
		case paper.DownloadStatusSuccess:
			succeeded++
		case paper.DownloadStatusFailed:
			failed++
		}
	}
	entries := make([]paper.DownloadEntryResult, len(j.entries))
	copy(entries, j.entries)
	return paper.DownloadJobSnapshot{
		JobID:       j.id,
		Total:       j.total,
		Succeeded:   succeeded,
		Failed:      failed,
		Completed:   completed,
		CompletedAt: completedAt,
		Entries:     entries,
	}
}

// runEntry runs one download attempt on the registry-owned background
// context and returns the classified result. The retry/backoff seam is
// marked in-line above the Ensure call.
func (r *Registry) runEntry(req paper.PDFDownloadRequest) paper.DownloadEntryResult {
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
		// Best-effort size read; a stat failure leaves Bytes=0 rather
		// than demoting an otherwise-successful download.
		if fi, statErr := os.Stat(loc.Path()); statErr == nil {
			result.Bytes = int(fi.Size())
		}
	}

	return result
}

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
