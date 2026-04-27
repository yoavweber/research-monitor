package route

import (
	arxivapp "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
	arxivctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/arxiv"
)

// ArxivRouter wires the arxiv fetch endpoint. It builds the use case and
// controller locally from the feature-scoped ArxivConfig, the shared
// paper.Repository (sourced from PaperConfig so the same persisted catalogue
// backs the fetch+persist orchestrator and the read endpoints), plus the
// shared Logger and Clock. Auth for this endpoint is supplied by the
// APIToken middleware already mounted on the /api group (requirement 1.2).
func ArxivRouter(d Deps) {
	uc := arxivapp.NewArxivUseCase(d.Arxiv.Fetcher, d.Paper.Repo, d.Logger, d.Arxiv.Query)
	ctrl := arxivctrl.NewArxivController(uc, d.Clock)

	g := d.Group.Group("/arxiv")
	g.GET("/fetch", ctrl.Fetch)
}
