// Package analyzer is the application-layer orchestrator for the llm-analyzer
// feature. It composes extraction.Repository, shared.LLMClient, and
// analyzer.Repository into the synchronous Analyze flow: load the extraction,
// gate on done status, run three sequential LLM calls, parse the thesis
// envelope, and upsert. Failures fail fast with typed sentinels — no retries
// at this layer.
package analyzer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	extraction "github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

type analyzerUseCase struct {
	repo     domain.Repository
	extracts extraction.Repository
	llm      shared.LLMClient
	logger   shared.Logger
	clock    shared.Clock
}

// NewAnalyzerUseCase wires the analyzer use case. The returned UseCase is
// safe for concurrent use as long as every collaborator is.
func NewAnalyzerUseCase(
	repo domain.Repository,
	extracts extraction.Repository,
	llm shared.LLMClient,
	logger shared.Logger,
	clock shared.Clock,
) domain.UseCase {
	return &analyzerUseCase{
		repo:     repo,
		extracts: extracts,
		llm:      llm,
		logger:   logger,
		clock:    clock,
	}
}

// Analyze drives the synchronous three-call orchestration. Errors map to
// typed sentinels in domain/analyzer so the HTTP layer's existing
// ErrorEnvelope middleware translates them onto the wire.
func (u *analyzerUseCase) Analyze(ctx context.Context, extractionID string) (*domain.Analysis, error) {
	row, err := u.extracts.FindByID(ctx, extractionID)
	if err != nil {
		if errors.Is(err, extraction.ErrNotFound) {
			return nil, domain.ErrExtractionNotFound
		}
		return nil, fmt.Errorf("%w: load extraction: %v", domain.ErrCatalogueUnavailable, err)
	}
	if row.Status != extraction.JobStatusDone || row.Artifact == nil {
		return nil, domain.ErrExtractionNotReady
	}
	body := row.Artifact.BodyMarkdown

	shortText, _, err := u.runCompletion(ctx, PromptVersionShort, promptShortSystem, body)
	if err != nil {
		return nil, err
	}
	longText, _, err := u.runCompletion(ctx, PromptVersionLong, promptLongSystem, body)
	if err != nil {
		return nil, err
	}
	thesisText, thesisModel, err := u.runCompletion(ctx, PromptVersionThesis, promptThesisSystem, body)
	if err != nil {
		return nil, err
	}

	thesis, ok := parseThesisEnvelope(thesisText)
	if !ok {
		u.logger.WarnContext(ctx, "analyzer.thesis_envelope_malformed",
			"extraction_id", extractionID,
			"raw_preview", previewN(thesisText, 256),
		)
		return nil, domain.ErrAnalyzerMalformedResponse
	}

	now := u.clock.Now()
	analysis := domain.Analysis{
		ExtractionID:         extractionID,
		ShortSummary:         strings.TrimSpace(shortText),
		LongSummary:          strings.TrimSpace(longText),
		ThesisAngleFlag:      thesis.Flag,
		ThesisAngleRationale: thesis.Rationale,
		Model:                thesisModel,
		PromptVersion:        PromptVersionComposite,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	persisted, err := u.repo.Upsert(ctx, analysis)
	if err != nil {
		return nil, err
	}

	u.logger.InfoContext(ctx, "analyzer.analyze.done",
		"extraction_id", extractionID,
		"prompt_version", persisted.PromptVersion,
		"model", persisted.Model,
		"thesis_flag", persisted.ThesisAngleFlag,
	)
	return &persisted, nil
}

// Get is a read-only retrieval; it never invokes the LLM.
func (u *analyzerUseCase) Get(ctx context.Context, extractionID string) (*domain.Analysis, error) {
	return u.repo.FindByID(ctx, extractionID)
}

// runCompletion is the single shared call site for shared.LLMClient.Complete.
// Returns the response text and the response's Model field on success, or
// ErrLLMUpstream on transport failure.
func (u *analyzerUseCase) runCompletion(ctx context.Context, version, systemPrompt, userPrompt string) (string, string, error) {
	resp, err := u.llm.Complete(ctx, shared.LLMRequest{
		SystemPrompt:  systemPrompt,
		UserPrompt:    userPrompt,
		PromptVersion: version,
	})
	if err != nil {
		u.logger.WarnContext(ctx, "analyzer.llm_upstream",
			"prompt_version", version,
			"error", err.Error(),
		)
		return "", "", fmt.Errorf("%w: %v", domain.ErrLLMUpstream, err)
	}
	if resp == nil {
		return "", "", fmt.Errorf("%w: nil response", domain.ErrLLMUpstream)
	}
	return resp.Text, resp.Model, nil
}

func previewN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
