package pdfdownload

import "github.com/yoavweber/research-monitor/backend/internal/domain/paper"

// JobCompletedForTest reports whether the worker for jobID has marked the
// job complete. Exposed only via the _test.go build tag so production
// callers cannot depend on this state — the supported observation path is
// the public PDFDownloadReader port (task 2.4).
func (r *Registry) JobCompletedForTest(jobID paper.DownloadJobID) bool {
	r.registryMu.Lock()
	j, ok := r.jobs[jobID]
	r.registryMu.Unlock()
	if !ok {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.completed
}

// JobEntriesForTest returns a copy of the per-entry results recorded by
// the worker so far. Same boundary rationale as JobCompletedForTest:
// tests need to observe progress before the public read port exists.
func (r *Registry) JobEntriesForTest(jobID paper.DownloadJobID) []paper.DownloadEntryResult {
	r.registryMu.Lock()
	j, ok := r.jobs[jobID]
	r.registryMu.Unlock()
	if !ok {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]paper.DownloadEntryResult, len(j.entries))
	copy(out, j.entries)
	return out
}

// JobEventsForTest returns a copy of the event log recorded so far. Used
// by 2.3 tests to assert the events log shape (Progress per entry + final
// Summary) before the public Subscribe contract lands in 2.4.
func (r *Registry) JobEventsForTest(jobID paper.DownloadJobID) []paper.DownloadEvent {
	r.registryMu.Lock()
	j, ok := r.jobs[jobID]
	r.registryMu.Unlock()
	if !ok {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]paper.DownloadEvent, len(j.events))
	copy(out, j.events)
	return out
}

// AttachTestSubscriberForJob registers a buffered channel as a subscriber
// of jobID under the per-job mutex and returns the channel for the test
// to read from. Mirrors the locking discipline that task 2.4's public
// SubscribePDFDownloadJob will use, without yet exposing the public
// (backlog, live) shape. Returns paper.ErrDownloadJobUnknown if the job
// is not present.
//
// The buffer size is supplied explicitly so the slow-subscriber test can
// force overflow with a small value, independent of the registry's
// configured SubscriberBuffer.
func (r *Registry) AttachTestSubscriberForJob(jobID paper.DownloadJobID, bufSize int) (chan paper.DownloadEvent, error) {
	r.registryMu.Lock()
	j, ok := r.jobs[jobID]
	r.registryMu.Unlock()
	if !ok {
		return nil, paper.ErrDownloadJobUnknown
	}
	ch := make(chan paper.DownloadEvent, bufSize)
	j.mu.Lock()
	j.subscribers = append(j.subscribers, ch)
	j.mu.Unlock()
	return ch, nil
}
