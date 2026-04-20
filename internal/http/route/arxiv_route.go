package route

import (
	arxivapp "github.com/yoavweber/defi-monitor-backend/internal/application/arxiv"
	arxivctrl "github.com/yoavweber/defi-monitor-backend/internal/http/controller/arxiv"
)

// ArxivRouter wires the arxiv fetch endpoint. It builds the use case and
// controller locally from the feature-scoped ArxivConfig plus the shared
// Logger and Clock from route.Deps, then registers GET /arxiv/fetch on the
// provided /api group. Auth for this endpoint is supplied by the APIToken
// middleware already mounted on the /api group (requirement 1.2).
func ArxivRouter(d Deps) {
	uc := arxivapp.NewArxivUseCase(d.Arxiv.Fetcher, d.Logger, d.Arxiv.Query)
	ctrl := arxivctrl.NewArxivController(uc, d.Clock)

	g := d.Group.Group("/arxiv")
	g.GET("/fetch", ctrl.Fetch)
}
