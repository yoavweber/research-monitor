// Package analyzer is the application-layer orchestrator for the llm-analyzer
// feature: load extraction, gate on done status, run three sequential LLM
// calls, parse the thesis envelope, upsert. Failures fail fast with typed
// sentinels — no retries at this layer.
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

	shortText, err := u.complete(ctx, PromptVersionShort, promptShortSystem, body)
	if err != nil {
		return nil, err
	}
	longText, err := u.complete(ctx, PromptVersionLong, promptLongSystem, body)
	if err != nil {
		return nil, err
	}
	thesisResp, err := u.llm.Complete(ctx, shared.LLMRequest{
		SystemPrompt:  promptThesisSystem,
		UserPrompt:    body,
		PromptVersion: PromptVersionThesis,
	})
	if err != nil {
		u.logger.WarnContext(ctx, "analyzer.llm_upstream",
			"prompt_version", PromptVersionThesis,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("%w: %v", domain.ErrLLMUpstream, err)
	}
	if thesisResp == nil {
		return nil, fmt.Errorf("%w: nil response", domain.ErrLLMUpstream)
	}

	thesis, ok := parseThesisEnvelope(thesisResp.Text)
	if !ok {
		u.logger.WarnContext(ctx, "analyzer.thesis_envelope_malformed",
			"extraction_id", extractionID,
			"raw_preview", truncate(thesisResp.Text, 256),
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
		Model:                thesisResp.Model,
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

func (u *analyzerUseCase) Get(ctx context.Context, extractionID string) (*domain.Analysis, error) {
	return u.repo.FindByID(ctx, extractionID)
}

// complete is the short/long shared call site; the thesis call needs the
// response's Model, so it's invoked inline by Analyze.
func (u *analyzerUseCase) complete(ctx context.Context, version, systemPrompt, userPrompt string) (string, error) {
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
		return "", fmt.Errorf("%w: %v", domain.ErrLLMUpstream, err)
	}
	if resp == nil {
		return "", fmt.Errorf("%w: nil response", domain.ErrLLMUpstream)
	}
	return resp.Text, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
