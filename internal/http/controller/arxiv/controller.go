package arxiv

import (
	"net/http"

	"github.com/gin-gonic/gin"

	arxivapp "github.com/yoavweber/research-monitor/backend/internal/application/arxiv"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// ArxivController is the HTTP handler for GET /api/arxiv/fetch. It delegates
// to arxivapp.UseCase, never performs status-mapping on errors itself
// (the ErrorEnvelope middleware owns that), and owns the response wire shape
// — including the per-entry source + is_new fields surfaced from persistence.
type ArxivController struct {
	uc    arxivapp.UseCase
	clock shared.Clock
}

// NewArxivController wires the controller to its use case and the clock used
// to stamp FetchedAt on successful responses. Injecting the clock keeps the
// response deterministic in tests.
func NewArxivController(uc arxivapp.UseCase, clock shared.Clock) *ArxivController {
	return &ArxivController{uc: uc, clock: clock}
}

// Fetch handles GET /api/arxiv/fetch. It takes no body and no query params;
// auth is enforced at the /api group level by the pre-existing APIToken
// middleware. On use-case error we hand off to c.Error — the ErrorEnvelope
// middleware translates *shared.HTTPError sentinels (paper.ErrUpstream*,
// paper.ErrCatalogueUnavailable) into the final status and envelope.
func (ctrl *ArxivController) Fetch(c *gin.Context) {
	results, err := ctrl.uc.Fetch(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToFetchResponse(results, ctrl.clock.Now())))
}
