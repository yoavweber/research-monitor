// Package arxiv implements paper.UseCase for the arXiv source. It is a pure
// orchestrator: it invokes a paper.Fetcher, logs the outcome, and relays
// paper.* sentinels unchanged. It never inspects HTTP status codes, bytes,
// XML, URLs, or transport-level errors.
package arxiv

import (
	"context"
	"errors"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

// arxivUseCase is the thin orchestrator that satisfies paper.UseCase for the
// arXiv source. It holds an immutable copy of the paper.Query so every call
// against a given instance is deterministic at the domain boundary.
type arxivUseCase struct {
	fetcher paper.Fetcher
	log     shared.Logger
	query   paper.Query
}

// NewArxivUseCase returns a paper.UseCase for the arXiv source. The fetcher,
// logger, and query are all provided by the bootstrap layer; the use case
// holds an immutable copy of the query for deterministic per-call behavior.
func NewArxivUseCase(fetcher paper.Fetcher, log shared.Logger, query paper.Query) paper.UseCase {
	return &arxivUseCase{fetcher: fetcher, log: log, query: query}
}

// Fetch delegates to the configured paper.Fetcher exactly once. On success it
// logs the outcome and returns the entries. On any error it logs the failure
// category (resolved via the sentinel identity of err) and relays the error
// verbatim, so higher layers can map sentinels to HTTP status codes.
func (u *arxivUseCase) Fetch(ctx context.Context) ([]paper.Entry, error) {
	entries, err := u.fetcher.Fetch(ctx, u.query)
	if err != nil {
		u.log.WarnContext(ctx, "paper.fetch.failed",
			"source", "arxiv",
			"category", classify(err),
			"err", err)
		return nil, err
	}
	u.log.InfoContext(ctx, "paper.fetch.ok",
		"source", "arxiv",
		"count", len(entries),
		"categories", u.query.Categories)
	return entries, nil
}

// classify returns a stable log string for the failure category, based on the
// sentinel identity of err. It never produces sentinels and never inspects
// transport-level error types.
func classify(err error) string {
	switch {
	case errors.Is(err, paper.ErrUpstreamBadStatus):
		return "bad_status"
	case errors.Is(err, paper.ErrUpstreamMalformed):
		return "malformed"
	case errors.Is(err, paper.ErrUpstreamUnavailable):
		return "unavailable"
	default:
		return "unknown"
	}
}
