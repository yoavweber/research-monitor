// Package extraction implements the extraction.UseCase: the orchestrator that
// sits between the HTTP controller and the worker on top of the domain
// Repository, Extractor, and Normalize. It is the single translator from
// extractor errors to FailureReason — the mapping table is centralised here,
// not in the controller or adapter.
package extraction

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// extractionUseCase is the sole implementation of extraction.UseCase. The
// struct is unexported so callers depend on the domain port returned by the
// constructor.
type extractionUseCase struct {
	repo      extraction.Repository
	extractor extraction.Extractor
	logger    shared.Logger
	clock     shared.Clock
	wakeCh    chan<- struct{}
	maxWords  int
}

// NewExtractionUseCase wires the dependencies for the extraction orchestrator
// and returns the domain port. wakeCh is the send-only end of the worker's
// pickup channel; Submit signals it non-blockingly after a successful Upsert
// so the worker drains the new row promptly. maxWords is the post-normalize
// word-count threshold above which an extraction is rejected as too_large.
func NewExtractionUseCase(
	repo extraction.Repository,
	extractor extraction.Extractor,
	logger shared.Logger,
	clock shared.Clock,
	wakeCh chan<- struct{},
	maxWords int,
) extraction.UseCase {
	return &extractionUseCase{
		repo:      repo,
		extractor: extractor,
		logger:    logger,
		clock:     clock,
		wakeCh:    wakeCh,
		maxWords:  maxWords,
	}
}

// Submit creates or overwrites the extraction row keyed by
// (SourceType, SourceID), emits an extraction.reextract log line on the
// overwrite path so operators can correlate re-runs against prior failures,
// and signals the worker non-blockingly. Validation mirrors the controller-side
// SubmitRequest.Validate so non-HTTP entrypoints get the same guarantees.
func (u *extractionUseCase) Submit(ctx context.Context, payload extraction.RequestPayload) (extraction.SubmitResult, error) {
	// Empty / whitespace check first so an entirely-missing payload reports
	// the shape error rather than the source_type-value error.
	if strings.TrimSpace(payload.SourceType) == "" ||
		strings.TrimSpace(payload.SourceID) == "" ||
		strings.TrimSpace(payload.PDFPath) == "" {
		return extraction.SubmitResult{}, extraction.ErrInvalidRequest
	}
	// Re-validate source_type for non-HTTP entrypoints. The controller-side
	// SubmitRequest.Validate is the first gate; this is the second.
	if payload.SourceType != "paper" {
		return extraction.SubmitResult{}, extraction.ErrUnsupportedSourceType
	}

	id, prior, err := u.repo.Upsert(ctx, payload)
	if err != nil {
		return extraction.SubmitResult{}, err
	}

	if prior != nil {
		// Overwrite path: emit one structured log line carrying the prior
		// status / reason so operators can correlate re-submits with the
		// failure they were trying to fix.
		u.logger.InfoContext(ctx, "extraction.reextract",
			"id", id,
			"source_type", payload.SourceType,
			"source_id", payload.SourceID,
			"prior_status", string(prior.Status),
			"prior_failure_reason", string(prior.FailureReason),
		)
	}

	// Non-blocking wake signal: the wake channel is buffered, so a full
	// buffer means the worker already has a pending pickup and dropping the
	// signal here is harmless.
	select {
	case u.wakeCh <- struct{}{}:
	default:
	}

	return extraction.SubmitResult{ID: id, Status: extraction.JobStatusPending}, nil
}

// Get is a thin pass-through to the repository read path so callers depend on
// the use-case port rather than the persistence layer.
func (u *extractionUseCase) Get(ctx context.Context, id string) (*extraction.Extraction, error) {
	return u.repo.FindByID(ctx, id)
}

// Process drives a single already-running row through extract → normalize →
// max-words gate → MarkDone or MarkFailed. The worker has already peeked,
// evaluated the pickup-time expiry predicate, and called ClaimPending — the
// row arrives in JobStatusRunning.
//
// On ctx cancellation mid-extraction (graceful shutdown), Process does NOT
// call MarkFailed. The row is left in running and the next-boot
// RecoverRunningOnStartup pass flips it to failed: process_restart. This
// keeps shutdown invariant-preserving (Critical Issue 1 resolution).
func (u *extractionUseCase) Process(ctx context.Context, row extraction.Extraction) error {
	output, err := u.extractor.Extract(ctx, extraction.ExtractInput{
		PDFPath:    row.RequestPayload.PDFPath,
		SourceType: row.RequestPayload.SourceType,
		SourceID:   row.RequestPayload.SourceID,
	})

	if err != nil {
		// ctx cancellation must short-circuit BEFORE any error→FailureReason
		// classification. We check ctx.Err() rather than errors.Is on a
		// specific cancellation flavour because the extractor may wrap the
		// underlying cause; the call-site ctx is the canonical signal.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reason, message := classifyExtractorError(err)
		if reason == extraction.FailureReasonExtractorFailure && !isKnownExtractorError(err) {
			u.logger.WarnContext(ctx, "extraction.worker.unexpected_error",
				"id", row.ID,
				"error", err.Error(),
			)
		}
		if markErr := u.repo.MarkFailed(ctx, row.ID, reason, message); markErr != nil {
			return fmt.Errorf("mark_failed: %w", markErr)
		}
		u.logger.WarnContext(ctx, "extraction.worker.failed",
			"id", row.ID,
			"reason", string(reason),
			"message", message,
		)
		return nil
	}

	// Success path: normalize, gate on max_words, mirror content_type, persist.
	normalized := extraction.Normalize(output.Markdown, basenameWithoutExt(row.RequestPayload.PDFPath))

	if normalized.WordCount > u.maxWords {
		message := fmt.Sprintf("word count %d exceeds threshold %d", normalized.WordCount, u.maxWords)
		if markErr := u.repo.MarkFailed(ctx, row.ID, extraction.FailureReasonTooLarge, message); markErr != nil {
			return fmt.Errorf("mark_failed: %w", markErr)
		}
		u.logger.WarnContext(ctx, "extraction.worker.failed",
			"id", row.ID,
			"reason", string(extraction.FailureReasonTooLarge),
			"message", message,
		)
		return nil
	}

	artifact := extraction.Artifact{
		Title:        normalized.Title,
		BodyMarkdown: normalized.BodyMarkdown,
		Metadata: extraction.Metadata{
			// ContentType is mirrored from RequestPayload.SourceType per the
			// design's Requirement 3.8 — extractions inherit the source-type
			// label so downstream consumers can route on it.
			ContentType: row.RequestPayload.SourceType,
			WordCount:   normalized.WordCount,
		},
	}
	if err := u.repo.MarkDone(ctx, row.ID, artifact); err != nil {
		return fmt.Errorf("mark_done: %w", err)
	}
	u.logger.InfoContext(ctx, "extraction.worker.done",
		"id", row.ID,
		"word_count", normalized.WordCount,
	)
	return nil
}

// classifyExtractorError is the centralised mapping table from Extractor
// typed errors to FailureReason. Anything outside the documented taxonomy is
// folded into extractor_failure — defensive for forward compatibility, while
// still surfacing as a typed terminal failure.
func classifyExtractorError(err error) (extraction.FailureReason, string) {
	switch {
	case errors.Is(err, extraction.ErrScannedPDF):
		return extraction.FailureReasonScannedPDF, err.Error()
	case errors.Is(err, extraction.ErrParseFailed):
		return extraction.FailureReasonParseFailed, err.Error()
	case errors.Is(err, extraction.ErrExtractorFailure):
		return extraction.FailureReasonExtractorFailure, err.Error()
	default:
		return extraction.FailureReasonExtractorFailure, err.Error()
	}
}

// isKnownExtractorError reports whether err matches one of the documented
// Extractor sentinels. The default branch in classifyExtractorError uses
// extractor_failure for unknown errors; this predicate lets the use case log
// a warning when that path fires so operators can spot extractor regressions.
func isKnownExtractorError(err error) bool {
	return errors.Is(err, extraction.ErrScannedPDF) ||
		errors.Is(err, extraction.ErrParseFailed) ||
		errors.Is(err, extraction.ErrExtractorFailure)
}

// basenameWithoutExt returns filepath.Base(p) with its extension stripped.
// Used as the Normalize fallbackTitle when the body has no level-1 heading.
func basenameWithoutExt(p string) string {
	base := filepath.Base(p)
	ext := filepath.Ext(base)
	if ext == "" {
		return base
	}
	return strings.TrimSuffix(base, ext)
}
