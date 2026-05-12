package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
)

// PDFDownloadScheduler is a hand-written fake of the consumer-defined
// scheduler interface (arxiv.DownloadScheduler). It records every call
// and returns a configurable snapshot or error so arxiv use case tests
// can assert "Schedule called once with these requests" without
// standing up the real registry.
//
// Default behavior: Schedule with a non-empty requests slice returns
// Snapshot if non-zero (tests typically set JobID and Total), otherwise
// auto-builds a minimal snapshot (Total = len(reqs), every entry
// Pending). Empty requests always returns a zero snapshot and nil error
// to match the production contract.
type PDFDownloadScheduler struct {
	mu sync.Mutex

	// Snapshot, if non-zero, is returned verbatim from Schedule. A zero
	// JobID disables this and the fake auto-builds a minimal snapshot
	// from the request slice.
	Snapshot pdfdownload.JobSnapshot

	// Err, if non-nil, is returned from Schedule.
	Err error

	// Calls records every set of requests received by Schedule, in call
	// order.
	Calls [][]pdfdownload.Request
}

// NewPDFDownloadScheduler returns a fresh scheduler fake.
func NewPDFDownloadScheduler() *PDFDownloadScheduler {
	return &PDFDownloadScheduler{}
}

// Schedule records the request slice (copied so caller mutations don't
// bleed into recorded state) and returns the configured Snapshot/Err,
// or a synthesized minimal snapshot when Snapshot is zero.
func (s *PDFDownloadScheduler) Schedule(_ context.Context, requests []pdfdownload.Request) (pdfdownload.JobSnapshot, error) {
	cp := make([]pdfdownload.Request, len(requests))
	copy(cp, requests)

	s.mu.Lock()
	s.Calls = append(s.Calls, cp)
	snapshot := s.Snapshot
	err := s.Err
	s.mu.Unlock()

	if err != nil {
		return pdfdownload.JobSnapshot{}, err
	}
	if len(requests) == 0 {
		return pdfdownload.JobSnapshot{}, nil
	}
	if snapshot.JobID != "" {
		return snapshot, nil
	}

	entries := make([]pdfdownload.EntryResult, 0, len(requests))
	for _, r := range requests {
		entries = append(entries, pdfdownload.EntryResult{
			PaperID: r.PaperID,
			Status:  pdfdownload.StatusPending,
		})
	}
	return pdfdownload.JobSnapshot{
		JobID:   "fake-job-id",
		Total:   len(requests),
		Entries: entries,
	}, nil
}

// CallCount is a convenience for tests that just want a count.
func (s *PDFDownloadScheduler) CallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Calls)
}

// LastCall returns the most recent request slice, or nil if Schedule
// has not been called.
func (s *PDFDownloadScheduler) LastCall() []pdfdownload.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Calls) == 0 {
		return nil
	}
	return s.Calls[len(s.Calls)-1]
}
