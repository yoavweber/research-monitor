package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
)

// PDFDownloadReader is a hand-written fake of the consumer-defined
// reader interface (paperctrl.PDFDownloadReader). It records every
// Snapshot and Subscribe call (method + job id) and returns
// programmable Snapshot / Subscribe payloads.
//
// Default behavior with no programming: Snapshot returns a zero
// JobSnapshot and pdfdownload.ErrJobUnknown; Subscribe returns nil
// backlog, a nil live channel, and pdfdownload.ErrJobUnknown. The fake
// therefore models the unknown-job path out-of-the-box and tests
// override SnapshotResult/Subscribe fields to model known jobs.
type PDFDownloadReader struct {
	mu sync.Mutex

	// SnapshotResult is returned verbatim from Snapshot when SnapshotErr
	// is nil. Tests set this to model a known job. (Field renamed from
	// "Snapshot" to avoid colliding with the method of the same name
	// required by the consumer interface.)
	SnapshotResult pdfdownload.JobSnapshot

	// SnapshotErr, if non-nil, is returned in place of SnapshotResult.
	// Default: pdfdownload.ErrJobUnknown (so the zero-value reader
	// already models an unknown job).
	SnapshotErr error

	// SubscribeBacklog, SubscribeLive, SubscribeErr program the
	// Subscribe response.
	SubscribeBacklog []pdfdownload.Event
	SubscribeLive    chan pdfdownload.Event
	SubscribeErr     error

	// SnapshotCalls and SubscribeCalls record the job ids each method
	// was invoked with, in call order.
	SnapshotCalls  []pdfdownload.JobID
	SubscribeCalls []pdfdownload.JobID
}

// NewPDFDownloadReader returns a reader fake whose default behavior
// models an unknown job for both Snapshot and Subscribe.
func NewPDFDownloadReader() *PDFDownloadReader {
	return &PDFDownloadReader{
		SnapshotErr:  pdfdownload.ErrJobUnknown,
		SubscribeErr: pdfdownload.ErrJobUnknown,
	}
}

// Snapshot records the call and returns the configured SnapshotResult /
// SnapshotErr.
func (r *PDFDownloadReader) Snapshot(_ context.Context, id pdfdownload.JobID) (pdfdownload.JobSnapshot, error) {
	r.mu.Lock()
	r.SnapshotCalls = append(r.SnapshotCalls, id)
	snap := r.SnapshotResult
	err := r.SnapshotErr
	r.mu.Unlock()

	if err != nil {
		return pdfdownload.JobSnapshot{}, err
	}
	return snap, nil
}

// Subscribe records the call and returns the configured backlog / live
// channel / err.
func (r *PDFDownloadReader) Subscribe(_ context.Context, id pdfdownload.JobID) ([]pdfdownload.Event, <-chan pdfdownload.Event, error) {
	r.mu.Lock()
	r.SubscribeCalls = append(r.SubscribeCalls, id)
	backlog := r.SubscribeBacklog
	live := r.SubscribeLive
	err := r.SubscribeErr
	r.mu.Unlock()

	if err != nil {
		return nil, nil, err
	}
	return backlog, live, nil
}

// SnapshotCallCount is a convenience for tests that want a count
// without juggling the mutex by hand.
func (r *PDFDownloadReader) SnapshotCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.SnapshotCalls)
}

// LastSnapshotCall returns the most recent job id passed to Snapshot,
// or the empty id if Snapshot was never called.
func (r *PDFDownloadReader) LastSnapshotCall() pdfdownload.JobID {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.SnapshotCalls) == 0 {
		return ""
	}
	return r.SnapshotCalls[len(r.SnapshotCalls)-1]
}
