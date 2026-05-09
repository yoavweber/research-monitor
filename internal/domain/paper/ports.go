package paper

import "context"

// Query is the immutable, source-neutral fetch criterion passed to any
// paper.Fetcher implementation. Validation (non-empty Categories, MaxResults
// within the allowed range) is enforced by the bootstrap layer at
// construction time; the struct itself does not re-validate.
type Query struct {
	Categories []string
	MaxResults int
}

// Fetcher — source-neutral domain-level fetch port. Implementations translate
// a Query into a source-specific call, execute it, and return typed Entry
// values or a paper.* sentinel. Concrete impls live in infrastructure/<source>/.
type Fetcher interface {
	Fetch(ctx context.Context, q Query) ([]Entry, error)
}

// Repository — persistence port for Entry values. Implementations live in
// infrastructure/<storage>/. The port is source-neutral; callers supply the
// (Source, SourceID) composite key explicitly.
type Repository interface {
	// Save persists an entry or reports it as skipped on composite-key collision.
	// DEDUPE: isNew=true indicates a new insert; isNew=false paired with err=nil
	// indicates a dedupe skip (the (Source, SourceID) pair was already present).
	// A non-nil err must be or wrap a *shared.HTTPError sentinel (today:
	// paper.ErrCatalogueUnavailable), so shared.AsHTTPError/errors.As can detect it.
	Save(ctx context.Context, e Entry) (isNew bool, err error)

	// FindByKey returns the stored entry or paper.ErrNotFound. On any other
	// storage failure, returns paper.ErrCatalogueUnavailable.
	FindByKey(ctx context.Context, source, sourceID string) (*Entry, error)

	// List returns every persisted entry, newest-first by SubmittedAt.
	// Empty result is a non-nil empty slice. On storage failure, returns
	// paper.ErrCatalogueUnavailable.
	List(ctx context.Context) ([]Entry, error)
}

// PDFScheduler is the write-side port for dispatching a batch of
// PDF downloads. The implementation lives in application/pdfdownload.
//
// Contract:
//   - SchedulePDFDownloads mutates registry state under the registry's
//     own mutex and DOES NOT consult ctx for that mutation. Once it
//     returns, the job is registered atomically and the worker has been
//     launched on a registry-owned background context. A client cancel
//     of ctx during the (very small) registration window cannot lose a
//     scheduled job.
//   - With an empty requests slice, the call returns a zero-value
//     DownloadJobSnapshot and a nil error; no job is created.
//   - On success the returned snapshot has Total set, Entries populated
//     with one DownloadStatusPending result per request (in submission
//     order), and Completed false.
//   - The ctx parameter is preserved for tracing and to satisfy the
//     project rule that context.Context is the first parameter of every
//     port method, but it never short-circuits registration.
//   - A non-nil error is returned only when the registry is shutting
//     down; in that case no job is registered.
type PDFScheduler interface {
	SchedulePDFDownloads(ctx context.Context, requests []PDFDownloadRequest) (DownloadJobSnapshot, error)
}

// PDFDownloadReader is the read-side port consumed by HTTP controllers.
//
// Contract:
//   - SnapshotPDFDownloadJob returns ErrDownloadJobUnknown when the job
//     id is unknown or has been evicted past its retention window.
//   - SubscribePDFDownloadJob captures the current event log into the
//     returned backlog slice and registers the live channel atomically
//     under the same per-job lock. Once it returns, every event
//     produced by the worker is delivered through exactly one of
//     backlog or live — never both, never neither.
//   - The live channel is closed after the terminal Summary event is
//     delivered, or when the subscriber is dropped under the
//     slow-consumer policy enforced by the implementation.
type PDFDownloadReader interface {
	SnapshotPDFDownloadJob(ctx context.Context, id DownloadJobID) (DownloadJobSnapshot, error)
	SubscribePDFDownloadJob(ctx context.Context, id DownloadJobID) (backlog []DownloadEvent, live <-chan DownloadEvent, err error)
}
