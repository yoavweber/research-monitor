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
