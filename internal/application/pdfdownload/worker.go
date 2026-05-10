package pdfdownload

import (
	"os"
	"time"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/pdf"
)

// runJob drives a single download job to completion. It is launched as a
// goroutine from SchedulePDFDownloads and runs entirely on the registry's
// background context — never the caller's ctx (R3.5). A failure on any
// entry is classified and recorded, but the loop continues so a single
// bad URL cannot starve the rest of the batch (R3.3).
//
// Task 2.3 extends the per-entry critical section: the entry result is
// appended to j.entries AND a DownloadEvent{Progress: &result} is appended
// to j.events AND fanned out to subscribers under j.mu. After the loop,
// the worker builds the final DownloadJobSnapshot, appends a
// DownloadEvent{Summary: &snap}, fans it out, closes every remaining
// subscriber channel, and flips j.completed/j.completedAt. Closing the
// channel after Summary is what tells well-behaved subscribers the
// stream is done (R4.3).
func (r *Registry) runJob(j *job) {
	defer r.workers.Done()

	startedAt := r.clock.Now()
	j.mu.Lock()
	j.startedAt = startedAt
	j.mu.Unlock()

	for i, req := range j.requests {
		result := r.runEntry(j.id, req)

		// Critical section: append entry, append Progress event, and
		// fan out to subscribers atomically. Releasing j.mu between
		// these three steps would let a new subscriber slot between
		// the append and the fan-out, breaking the atomic-with-append
		// invariant (R4.1 / R5.4). Subscribers attached after this
		// section will pick up the event from the events log via the
		// public Subscribe contract (task 2.4).
		ev := paper.DownloadEvent{JobID: j.id, Progress: &result}
		dropped := r.appendAndFanOut(j, i, result, ev)
		// Logging is outside the per-job mutex to keep the critical
		// section tight and to avoid holding j.mu while the logger
		// formats. The entry-completed log shape is unchanged from 2.2.
		r.emitEntryCompleted(j.id, result)
		r.emitSubscriberDropped(j.id, dropped)
	}

	// Completion: build summary, append it, fan it out, close remaining
	// subscribers, flip the completion flag. All under j.mu so no other
	// event can sneak in after Summary on any subscriber.
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

// emitSubscriberDropped writes one Warn record per dropped subscriber.
// Lives on the registry so the worker keeps a single shape for lifecycle
// logging and the call site stays uncluttered.
func (r *Registry) emitSubscriberDropped(jobID paper.DownloadJobID, count int) {
	for range count {
		r.logger.WarnContext(r.bgCtx, "pdfdownload.subscriber.dropped",
			"job_id", string(jobID),
			"reason", "slow_consumer",
		)
	}
}

// appendAndFanOut commits one entry result to the job state and fans out
// the corresponding Progress event to every live subscriber. The non-
// blocking-send pattern is the worker's safety net: a subscriber that is
// not draining its channel is closed and removed instead of stalling the
// worker (R4.4).
//
// Returns the number of subscribers dropped on this call so the caller
// can emit one pdfdownload.subscriber.dropped log per drop outside the
// critical section.
func (r *Registry) appendAndFanOut(j *job, idx int, result paper.DownloadEntryResult, ev paper.DownloadEvent) int {
	j.mu.Lock()
	defer j.mu.Unlock()

	// Overwrite the pre-seeded Pending entry at the same index so the
	// entries slice mirrors j.requests by position. Downstream snapshot
	// projection (task 2.4) relies on this ordering.
	j.entries[idx] = result
	j.events = append(j.events, ev)
	dropped := fanOutLocked(j, ev)
	return dropped
}

// finishJob is the post-loop critical section. It builds the final
// DownloadJobSnapshot from current state, appends a Summary event,
// fans it out, closes every remaining subscriber channel, and sets
// completed/completedAt. Returns the count of subscribers dropped during
// the summary fan-out plus the snapshot so the caller can log
// pdfdownload.job.completed with the final counts.
//
// Closing the subscriber channels after Summary is delivered is the
// signal to well-behaved subscribers that the stream is done; the
// controller side then ends its SSE response (R4.3).
func (r *Registry) finishJob(j *job, completedAt time.Time) (dropped int, snapshot paper.DownloadJobSnapshot) {
	j.mu.Lock()
	defer j.mu.Unlock()

	snap := buildSnapshotLocked(j, completedAt)
	ev := paper.DownloadEvent{JobID: j.id, Summary: &snap}
	j.events = append(j.events, ev)
	dropped = fanOutLocked(j, ev)

	// Close every subscriber still attached (the ones that did not get
	// dropped during this same fan-out). fanOutLocked has already removed
	// dropped subscribers from j.subscribers, so the remaining slice
	// holds only the well-behaved ones.
	for _, ch := range j.subscribers {
		close(ch)
	}
	j.subscribers = nil

	j.completed = true
	j.completedAt = completedAt

	return dropped, snap
}

// fanOutLocked sends ev to each subscriber using a non-blocking send.
// Subscribers whose channels are full are closed and removed from the
// subscribers slice; their slots are not preserved. Must be called with
// j.mu held.
//
// The select{default} pattern is mandatory (design.md): a stalled
// consumer must never block the worker.
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
	// Zero out the tail of the original slice so closed channels are
	// not retained by the underlying array.
	for i := len(kept); i < len(j.subscribers); i++ {
		j.subscribers[i] = nil
	}
	j.subscribers = kept
	return dropped
}

// buildSnapshotLocked builds a DownloadJobSnapshot from the current job
// state. Must be called with j.mu held. Counts succeeded/failed by
// walking the entries slice; this is the single source of truth at
// completion time. Entries is copied so the snapshot is safe to hand to
// callers outside the critical section.
func buildSnapshotLocked(j *job, completedAt time.Time) paper.DownloadJobSnapshot {
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
		Completed:   true,
		CompletedAt: completedAt,
		Entries:     entries,
	}
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
