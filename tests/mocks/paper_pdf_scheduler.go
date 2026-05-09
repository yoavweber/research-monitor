package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// Compile-time conformance — surfaces paper.PDFScheduler port drift at
// build time rather than at first use from a test.
var _ paper.PDFScheduler = (*PaperPDFScheduler)(nil)

// PaperPDFScheduler is a hand-written paper.PDFScheduler fake. It
// records every call and returns a configurable snapshot or error so
// arxiv use case tests can assert "Schedule called once with these
// requests" without standing up the real registry.
//
// Default behavior: SchedulePDFDownloads with a non-empty requests
// slice returns Snapshot if non-zero (tests typically set JobID and
// Total), otherwise auto-builds a minimal snapshot (Total = len(reqs),
// every entry Pending). Empty requests always returns a zero snapshot
// and nil error to match the production contract.
type PaperPDFScheduler struct {
	mu sync.Mutex

	// Snapshot, if non-zero, is returned verbatim from
	// SchedulePDFDownloads. A zero JobID disables this and the fake
	// auto-builds a minimal snapshot from the request slice.
	Snapshot paper.DownloadJobSnapshot

	// Err, if non-nil, is returned from SchedulePDFDownloads.
	Err error

	// Calls records every set of requests received by
	// SchedulePDFDownloads, in call order.
	Calls [][]paper.PDFDownloadRequest
}

// NewPaperPDFScheduler returns a fresh scheduler fake.
func NewPaperPDFScheduler() *PaperPDFScheduler {
	return &PaperPDFScheduler{}
}

// SchedulePDFDownloads satisfies paper.PDFScheduler. It records the
// request slice (copied so caller mutations don't bleed into recorded
// state) and returns the configured Snapshot/Err, or a synthesized
// minimal snapshot when Snapshot is zero.
func (s *PaperPDFScheduler) SchedulePDFDownloads(_ context.Context, requests []paper.PDFDownloadRequest) (paper.DownloadJobSnapshot, error) {
	cp := make([]paper.PDFDownloadRequest, len(requests))
	copy(cp, requests)

	s.mu.Lock()
	s.Calls = append(s.Calls, cp)
	snapshot := s.Snapshot
	err := s.Err
	s.mu.Unlock()

	if err != nil {
		return paper.DownloadJobSnapshot{}, err
	}
	if len(requests) == 0 {
		return paper.DownloadJobSnapshot{}, nil
	}
	if snapshot.JobID != "" {
		return snapshot, nil
	}

	entries := make([]paper.DownloadEntryResult, 0, len(requests))
	for _, r := range requests {
		entries = append(entries, paper.DownloadEntryResult{
			PaperID: r.PaperID,
			Status:  paper.DownloadStatusPending,
		})
	}
	return paper.DownloadJobSnapshot{
		JobID:   "fake-job-id",
		Total:   len(requests),
		Entries: entries,
	}, nil
}

// CallCount is a convenience for tests that just want a count.
func (s *PaperPDFScheduler) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Calls)
}

// LastCall returns the most recent request slice, or nil if Schedule
// has not been called.
func (s *PaperPDFScheduler) LastCall() []paper.PDFDownloadRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Calls) == 0 {
		return nil
	}
	return s.Calls[len(s.Calls)-1]
}
