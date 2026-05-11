package mocks

import (
	"context"
	"sync"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
)

// Compile-time conformance — surfaces paper.PDFDownloadReader port drift
// at build time rather than at first use from a test.
var _ paper.PDFDownloadReader = (*PaperPDFDownloadReader)(nil)

// PaperPDFDownloadReader is a hand-written paper.PDFDownloadReader fake.
// It records every Snapshot and Subscribe call (method + job id) and
// returns programmable Snapshot / Subscribe payloads.
//
// Default behavior with no programming: Snapshot returns a zero
// DownloadJobSnapshot and paper.ErrDownloadJobUnknown; Subscribe returns
// nil backlog, a nil live channel, and paper.ErrDownloadJobUnknown. The
// fake therefore models the unknown-job path out-of-the-box and tests
// override Snapshot/Subscribe fields to model known jobs.
//
// Subscribe is not exercised by task 3.1 but is included here so task
// 3.2 can reuse the same fake without churn (no new struct, no test
// rewires).
type PaperPDFDownloadReader struct {
	mu sync.Mutex

	// Snapshot is returned verbatim from SnapshotPDFDownloadJob when
	// SnapshotErr is nil. Tests set this to model a known job.
	Snapshot paper.DownloadJobSnapshot

	// SnapshotErr, if non-nil, is returned in place of Snapshot.
	// Default: paper.ErrDownloadJobUnknown (so the zero-value reader
	// already models an unknown job).
	SnapshotErr error

	// SubscribeBacklog, SubscribeLive, SubscribeErr program the
	// Subscribe response. Task 3.1 leaves all three at zero values.
	SubscribeBacklog []paper.DownloadEvent
	SubscribeLive    chan paper.DownloadEvent
	SubscribeErr     error

	// SnapshotCalls and SubscribeCalls record the job ids each method
	// was invoked with, in call order.
	SnapshotCalls  []paper.DownloadJobID
	SubscribeCalls []paper.DownloadJobID
}

// NewPaperPDFDownloadReader returns a reader fake whose default
// behavior models an unknown job for both Snapshot and Subscribe.
func NewPaperPDFDownloadReader() *PaperPDFDownloadReader {
	return &PaperPDFDownloadReader{
		SnapshotErr:  paper.ErrDownloadJobUnknown,
		SubscribeErr: paper.ErrDownloadJobUnknown,
	}
}

// SnapshotPDFDownloadJob satisfies paper.PDFDownloadReader. It records
// the call and returns the configured Snapshot / SnapshotErr.
func (r *PaperPDFDownloadReader) SnapshotPDFDownloadJob(_ context.Context, id paper.DownloadJobID) (paper.DownloadJobSnapshot, error) {
	r.mu.Lock()
	r.SnapshotCalls = append(r.SnapshotCalls, id)
	snap := r.Snapshot
	err := r.SnapshotErr
	r.mu.Unlock()

	if err != nil {
		return paper.DownloadJobSnapshot{}, err
	}
	return snap, nil
}

// SubscribePDFDownloadJob satisfies paper.PDFDownloadReader. It records
// the call and returns the configured backlog / live channel / err.
func (r *PaperPDFDownloadReader) SubscribePDFDownloadJob(_ context.Context, id paper.DownloadJobID) ([]paper.DownloadEvent, <-chan paper.DownloadEvent, error) {
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
func (r *PaperPDFDownloadReader) SnapshotCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.SnapshotCalls)
}

// LastSnapshotCall returns the most recent job id passed to
// SnapshotPDFDownloadJob, or the empty id if Snapshot was never called.
func (r *PaperPDFDownloadReader) LastSnapshotCall() paper.DownloadJobID {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.SnapshotCalls) == 0 {
		return ""
	}
	return r.SnapshotCalls[len(r.SnapshotCalls)-1]
}
