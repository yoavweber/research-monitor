package bootstrap

import (
	"github.com/yoavweber/research-monitor/backend/internal/domain/source"
)

// seedSources is the canonical list of sources every environment starts with.
// Idempotency is guaranteed by the URL-unique constraint enforced in
// sourceUseCase.Create — re-running the seed is a no-op on rows that already exist.
var seedSources = []source.CreateRequest{
	{
		Name: "arXiv",
		Kind: source.KindAPI,
		URL:  "https://export.arxiv.org/api/query",
	},
}
