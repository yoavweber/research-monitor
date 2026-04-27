// Package arxiv implements the arxiv-side fetch+persist orchestrator. It is a
// pure orchestrator: it invokes a paper.Fetcher, persists each returned entry
// through paper.Repository, and surfaces per-entry is_new results to its
// caller. It never inspects HTTP status codes, bytes, XML, URLs, or
// transport-level errors; paper.* sentinels are relayed verbatim.
package arxiv

import (
	"context"
	"errors"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
)

// Result pairs a fetched domain Entry with the per-entry persist outcome the
// HTTP layer surfaces in its response. IsNew is true when Save inserted a new
// row and false when Save short-circuited on a (Source, SourceID) collision.
//
// Lives in this package (not domain/paper) because IsNew is a per-fetch
// persistence artefact, not a property of the paper itself.
type Result struct {
	Entry paper.Entry
	IsNew bool
}

// UseCase is the arxiv application port. The HTTP controller depends on this
// narrow interface; arxivUseCase is its sole implementation. Defined here
// because Result is application-specific and does not belong in domain/paper.
type UseCase interface {
	Fetch(ctx context.Context) ([]Result, error)
}

// arxivUseCase orchestrates the arxiv fetch + per-entry persist sequence. It
// holds an immutable copy of paper.Query so every call against a given
// instance is deterministic at the domain boundary.
type arxivUseCase struct {
	fetcher paper.Fetcher
	repo    paper.Repository
	log     shared.Logger
	query   paper.Query
}

// NewArxivUseCase returns the arxiv UseCase. Fetcher, repository, logger, and
// query are all provided by the bootstrap layer.
func NewArxivUseCase(fetcher paper.Fetcher, repo paper.Repository, log shared.Logger, query paper.Query) UseCase {
	return &arxivUseCase{fetcher: fetcher, repo: repo, log: log, query: query}
}

// Fetch fetches once, then persists each returned entry in order, pairing
// each with its is_new result. Order is preserved exactly as produced by the
// fetcher (R5.7). On any save failure the loop aborts and returns
// (nil, saveErr) — no partial slice is leaked to the caller (R5.5).
func (u *arxivUseCase) Fetch(ctx context.Context) ([]Result, error) {
	entries, err := u.fetcher.Fetch(ctx, u.query)
	if err != nil {
		u.log.WarnContext(ctx, "paper.fetch.failed",
			"source", paper.SourceArxiv,
			"category", classify(err),
			"err", err)
		return nil, err
	}

	results := make([]Result, 0, len(entries))
	newCount, skippedCount := 0, 0
	// Per-entry Save → one SQLite-WAL fsync per row. Batching into a single
	// transaction (e.g. repo.SaveAll) would amortize the fsync cost across the
	// whole batch; revisit if /api/arxiv/fetch latency starts dominating.
	for _, e := range entries {
		isNew, saveErr := u.repo.Save(ctx, e)
		if saveErr != nil {
			// Repository already typed this as paper.ErrCatalogueUnavailable;
			// relay verbatim so the HTTP layer maps it to its sentinel status.
			u.log.ErrorContext(ctx, "paper.fetch.persist_failed",
				"source", paper.SourceArxiv,
				"source_id", e.SourceID,
				"err", saveErr)
			return nil, saveErr
		}
		results = append(results, Result{Entry: e, IsNew: isNew})
		if isNew {
			newCount++
		} else {
			skippedCount++
		}
	}

	u.log.InfoContext(ctx, "paper.fetch.ok",
		"source", paper.SourceArxiv,
		"count", len(entries),
		"new", newCount,
		"skipped", skippedCount,
		"categories", u.query.Categories)
	return results, nil
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
