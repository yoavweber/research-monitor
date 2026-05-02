// Package extraction implements extraction.UseCase: the orchestrator between
// the HTTP controller, the worker, and the domain Repository / Extractor /
// Normalize. It is the single translator from extractor errors to
// FailureReason; the mapping table is centralised here.
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

type extractionUseCase struct {
	repo      extraction.Repository
	extractor extraction.Extractor
	logger    shared.Logger
	clock     shared.Clock
	notifier  extraction.Notifier
	maxWords  int
}

func NewExtractionUseCase(
	repo extraction.Repository,
	extractor extraction.Extractor,
	logger shared.Logger,
	clock shared.Clock,
	notifier extraction.Notifier,
	maxWords int,
) extraction.UseCase {
	return &extractionUseCase{
		repo:      repo,
		extractor: extractor,
		logger:    logger,
		clock:     clock,
		notifier:  notifier,
		maxWords:  maxWords,
	}
}

func (u *extractionUseCase) Submit(ctx context.Context, payload extraction.RequestPayload) (extraction.SubmitResult, error) {
	req := extraction.SubmitRequest{
		SourceType: payload.SourceType,
		SourceID:   payload.SourceID,
		PDFPath:    payload.PDFPath,
	}
	if err := req.Validate(); err != nil {
		return extraction.SubmitResult{}, err
	}

	id, prior, err := u.repo.Upsert(ctx, payload)
	if err != nil {
		return extraction.SubmitResult{}, err
	}

	if prior != nil {
		u.logger.InfoContext(ctx, "extraction.reextract",
			"id", id,
			"source_type", payload.SourceType,
			"source_id", payload.SourceID,
			"prior_status", string(prior.Status),
			"prior_failure_reason", string(prior.FailureReason),
		)
	}

	u.notifier.Notify(ctx)

	return extraction.SubmitResult{ID: id, Status: extraction.JobStatusPending}, nil
}

func (u *extractionUseCase) Get(ctx context.Context, id string) (*extraction.Extraction, error) {
	return u.repo.FindByID(ctx, id)
}

// Process drives one already-running row through extract → normalize →
// max-words gate → MarkDone or MarkFailed. The worker has already peeked,
// evaluated expiry, and called ClaimPending — the row arrives in
// JobStatusRunning. On ctx cancellation mid-extraction Process returns
// without writing; the row stays in running and the next boot's
// RecoverRunningOnStartup flips it to failed: process_restart.
func (u *extractionUseCase) Process(ctx context.Context, row extraction.Extraction) error {
	output, err := u.extractor.Extract(ctx, extraction.ExtractInput{
		PDFPath:    row.RequestPayload.PDFPath,
		SourceType: row.RequestPayload.SourceType,
		SourceID:   row.RequestPayload.SourceID,
	})

	if err != nil {
		// ctx.Err() is checked before classifying err so a graceful shutdown
		// (where the extractor may wrap the cancellation cause) is never
		// misclassified as extractor_failure.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reason, message, known := classifyExtractorError(err)
		if !known {
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

	fallbackTitle := strings.TrimSuffix(filepath.Base(row.RequestPayload.PDFPath), filepath.Ext(row.RequestPayload.PDFPath))
	normalized := extraction.Normalize(output.Markdown, fallbackTitle)

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

// classifyExtractorError maps an Extractor typed error to a FailureReason and
// reports whether the error matched one of the documented sentinels. Unknown
// errors fold into extractor_failure with known=false so the caller can log a
// regression warning before the failure is persisted.
func classifyExtractorError(err error) (reason extraction.FailureReason, message string, known bool) {
	switch {
	case errors.Is(err, extraction.ErrScannedPDF):
		return extraction.FailureReasonScannedPDF, err.Error(), true
	case errors.Is(err, extraction.ErrParseFailed):
		return extraction.FailureReasonParseFailed, err.Error(), true
	case errors.Is(err, extraction.ErrExtractorFailure):
		return extraction.FailureReasonExtractorFailure, err.Error(), true
	default:
		return extraction.FailureReasonExtractorFailure, err.Error(), false
	}
}
