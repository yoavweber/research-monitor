package extraction

import "time"

// JobStatus is the lifecycle state of an Extraction row. Every extraction
// progresses through pending → running → done | failed (Requirement 5.1);
// done and failed are terminal.
type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

// FailureReason is the typed taxonomy of terminal failure causes. The six
// constants are mutually exclusive: any single extraction carries at most one
// reason (Requirement 4.5).
type FailureReason string

const (
	FailureReasonScannedPDF       FailureReason = "scanned_pdf"
	FailureReasonParseFailed      FailureReason = "parse_failed"
	FailureReasonExtractorFailure FailureReason = "extractor_failure"
	FailureReasonTooLarge         FailureReason = "too_large"
	FailureReasonExpired          FailureReason = "expired"
	FailureReasonProcessRestart   FailureReason = "process_restart"
)

// Metadata holds the post-normalization descriptors mirrored on a successful
// Artifact. ContentType mirrors RequestPayload.SourceType (Requirement 3.8);
// WordCount is the whitespace-token count of the normalized body
// (Requirement 3.7).
type Metadata struct {
	ContentType string
	WordCount   int
}

// Artifact is the successful output of an extraction: the normalized markdown
// body, a derived title, and the metadata block. Artifact is non-nil iff
// Extraction.Status == JobStatusDone.
type Artifact struct {
	Title        string
	BodyMarkdown string
	Metadata     Metadata
}

// RequestPayload is the worker's execution input for one extraction. It is
// JSON-encoded onto the row at submit time so the worker reads its inputs from
// the row alone.
type RequestPayload struct {
	// SourceType is the upstream domain aggregate (v1 only "paper").
	SourceType string
	// SourceID is the provider-local identifier within SourceType.
	SourceID string
	// PDFPath is the local filesystem path to the PDF the extractor will read.
	PDFPath string
}

// Failure describes the terminal failure of an extraction. Failure is non-nil
// iff Extraction.Status == JobStatusFailed.
type Failure struct {
	Reason  FailureReason
	Message string
}

// Extraction is the aggregate root. Identity is the row id (UUID); external
// keying is the composite (SourceType, SourceID). Invariants: Status ∈
// {pending, running, done, failed}; Artifact non-nil iff Status == done;
// Failure non-nil iff Status == failed.
type Extraction struct {
	ID             string
	SourceType     string
	SourceID       string
	Status         JobStatus
	RequestPayload RequestPayload
	Artifact       *Artifact
	Failure        *Failure
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// PriorState describes the row that was overwritten by Repository.Upsert.
// FailureReason is the zero value when the prior status was not failed.
type PriorState struct {
	Status        JobStatus
	FailureReason FailureReason
}

// NormalizedArtifact is the output of the pure Normalize function: a derived
// title, the normalized markdown body, and the whitespace-token word count.
type NormalizedArtifact struct {
	Title        string
	BodyMarkdown string
	WordCount    int
}

// ExtractInput is the request value passed to Extractor.Extract. SourceType
// and SourceID are forwarded as hints to extractors that may behave
// differently per source.
type ExtractInput struct {
	PDFPath    string
	SourceType string
	SourceID   string
}

// ExtractOutput is the success value returned by Extractor.Extract. Markdown
// is non-empty on success; TitleHint is informational only — the use case
// prefers the Normalize-derived title.
type ExtractOutput struct {
	Markdown  string
	TitleHint string
}

// SubmitResult is the value returned by UseCase.Submit. Status is always
// JobStatusPending on success.
type SubmitResult struct {
	ID     string
	Status JobStatus
}
