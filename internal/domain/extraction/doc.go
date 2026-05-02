// Package extraction is the source-neutral domain aggregate for converting a
// PDF on disk into normalized markdown plus minimal metadata. It owns the
// Extraction aggregate and its value types (Artifact, Metadata, RequestPayload,
// Failure, JobStatus, FailureReason, PriorState, NormalizedArtifact,
// ExtractInput, ExtractOutput, SubmitResult), the Repository / UseCase /
// Extractor ports, and the aggregate-scoped error sentinels.
//
// Identity convention: every extraction is keyed externally by the composite
// (source_type, source_id) pair. SourceType identifies the upstream domain
// aggregate (in v1 only "paper") and SourceID is that aggregate's
// provider-local id. The row id is an internal UUID generated at persist time.
//
// Dependency rule: this package may not import anything under
// internal/infrastructure/. Domain code depends on interfaces declared here;
// concrete adapters live under internal/infrastructure/ and are wired in
// bootstrap.
package extraction
