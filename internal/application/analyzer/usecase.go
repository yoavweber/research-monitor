// Package analyzer is the application-layer orchestrator: load the
// extraction, gate on done status, run two sequential LLM calls (short +
// long), upsert. Failures fail fast with typed sentinels — no retries.
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

// Placeholder values populated for the persisted thesis_angle_* columns
// until the thesis-classifier follow-up spec ships. The columns stay so the
// downstream wire shape doesn't churn when the real classifier lands.
const (
	thesisFlagPlaceholder      = true
	thesisRationalePlaceholder = "default — thesis classification not yet implemented; see thesis-profile follow-up spec."
)

type analyzerUseCase struct {
	repo     domain.Repository
	extracts extraction.Repository
	llm      shared.LLMClient
	logger   shared.Logger
	clock    shared.Clock
}

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

	shortText, _, err := u.complete(ctx, PromptVersionShort, promptShortSystem, body)
	if err != nil {
		return nil, err
	}
	longText, model, err := u.complete(ctx, PromptVersionLong, promptLongSystem, body)
	if err != nil {
		return nil, err
	}

	now := u.clock.Now()
	analysis := domain.Analysis{
		ExtractionID:         extractionID,
		ShortSummary:         strings.TrimSpace(shortText),
		LongSummary:          strings.TrimSpace(longText),
		ThesisAngleFlag:      thesisFlagPlaceholder,
		ThesisAngleRationale: thesisRationalePlaceholder,
		Model:                model,
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
	)
	return &persisted, nil
}

func (u *analyzerUseCase) Get(ctx context.Context, extractionID string) (*domain.Analysis, error) {
	return u.repo.FindByID(ctx, extractionID)
}

func (u *analyzerUseCase) complete(ctx context.Context, version, systemPrompt, userPrompt string) (string, string, error) {
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
