package extraction

import "context"

// Repository owns the on-disk shape of an extraction and the only writer of its
// rows. Implementations live under internal/infrastructure/persistence/extraction/.
// Every method receives a non-cancelled ctx and surfaces a precondition
// violation as ErrInvalidTransition rather than a silent no-op.
type Repository interface {
	// Upsert inserts a new pending row for payload, or — if a row with the same
	// (SourceType, SourceID) already exists — overwrites it: status reset to
	// pending, body / failure cleared, request_payload replaced, created_at
	// refreshed so the new request gets a full job_expiry window. The original
	// row id is preserved. priorStatus is non-nil iff a row was overwritten.
	Upsert(ctx context.Context, payload RequestPayload) (id string, priorStatus *PriorState, err error)

	// FindByID returns the extraction row keyed by id, or ErrNotFound.
	FindByID(ctx context.Context, id string) (*Extraction, error)

	// PeekNextPending returns the oldest pending row (oldest by created_at)
	// without transitioning its status. ok=false signals an empty queue. The
	// returned row's request_payload is the worker's execution input. The
	// worker calls this first so the pickup-time expiry predicate (Requirement
	// 5.2) is evaluated while the row is still pending — an expired row is
	// marked failed without ever entering running.
	PeekNextPending(ctx context.Context) (row *Extraction, ok bool, err error)

	// ClaimPending atomically transitions a specific row from pending to
	// running. Returns ErrInvalidTransition if the row's current status is not
	// pending (e.g. another writer claimed it; in v1 the single worker
	// guarantees this is unreachable, but the precondition is still enforced
	// at the storage layer for forward compatibility with multi-worker).
	ClaimPending(ctx context.Context, id string) error

	// MarkDone writes the artifact body, normalized metadata, and transitions
	// status to done. id MUST currently be in running. A zero-rows-affected
	// UPDATE surfaces as ErrInvalidTransition.
	MarkDone(ctx context.Context, id string, artifact Artifact) error

	// MarkFailed transitions status to failed with the given reason and message.
	// The allowed prior status depends on the reason:
	//   - reason == FailureReasonExpired         -> id MUST currently be in pending
	//   - any other FailureReason                -> id MUST currently be in running
	// Implementations enforce this in the UPDATE predicate so a misuse fails
	// loudly with ErrInvalidTransition rather than silently mutating a terminal row.
	MarkFailed(ctx context.Context, id string, reason FailureReason, message string) error

	// RecoverRunningOnStartup transitions every row currently in running to
	// failed: process_restart. Called from bootstrap before the worker goroutine
	// launches and before any HTTP request is served. The operation is
	// idempotent — a second call after the first observes recovered == 0.
	RecoverRunningOnStartup(ctx context.Context) (recovered int, err error)

	// ListPendingIDs returns the ids of every row currently in pending, oldest
	// first by created_at. Called once at startup so bootstrap can self-signal
	// the worker channel per pending row.
	ListPendingIDs(ctx context.Context) ([]string, error)
}

// UseCase orchestrates Submit (enqueue / re-enqueue), Get (read), and Process
// (worker step) on top of Repository, Extractor, Normalize, Logger, and Clock.
// It is the only translator from extractor errors to FailureReason — the
// mapping table is centralised here, not in the controller or adapter.
type UseCase interface {
	// Submit creates or overwrites the row for payload, sends a non-blocking
	// wake signal on the worker channel, and returns the id and current status
	// (always JobStatusPending on success). Returns ErrInvalidRequest when any
	// field of the payload is empty after trimming, and ErrUnsupportedSourceType
	// when SourceType is non-empty but not exactly "paper". On a re-enqueue
	// (prior row existed) the row's id is preserved and exactly one
	// extraction.reextract log line is emitted with the prior status / reason.
	Submit(ctx context.Context, payload RequestPayload) (SubmitResult, error)

	// Get returns the current state of an extraction or ErrNotFound.
	Get(ctx context.Context, id string) (*Extraction, error)

	// Process drives a single already-running row through extract → normalize →
	// max-words gate → MarkDone or MarkFailed. The worker guarantees the row
	// arrives in JobStatusRunning (it has already peeked, evaluated the
	// pickup-time expiry predicate, and called ClaimPending). When ctx is
	// cancelled mid-extraction (graceful shutdown), Process does not call
	// MarkFailed; the row is left in running for RecoverRunningOnStartup to
	// flip to failed: process_restart on the next boot.
	Process(ctx context.Context, row Extraction) error
}

// Extractor converts a PDF on disk into raw markdown plus a tool-emitted title
// hint, or a typed domain error from a fixed taxonomy. Implementations live
// under internal/infrastructure/extraction/<tool>/. Replacing the tool does
// not change the request, response, status, or failure-reason behaviour of
// the surrounding system (Requirement 6.1).
type Extractor interface {
	// Extract reads the PDF at in.PDFPath and returns either a non-empty
	// Markdown body or one of four error categories: ErrScannedPDF (no
	// extractable text), ErrParseFailed (corrupt / unparseable input),
	// ErrExtractorFailure (catch-all infrastructure failure), or a
	// ctx-cancellation error (ctx.Err()). Implementations must respect ctx
	// cancellation (subprocess kill on ctx.Done()) and clean up any temp
	// resources before returning, regardless of outcome.
	Extract(ctx context.Context, in ExtractInput) (ExtractOutput, error)
}
