package arxiv

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/common"
)

// ArxivController is the HTTP handler for GET /api/arxiv/fetch. It delegates
// to paper.UseCase, never performs status-mapping on errors itself (the
// ErrorEnvelope middleware owns that), and owns the response wire shape.
type ArxivController struct {
	uc    paper.UseCase
	clock shared.Clock
}

// NewArxivController wires the controller to its use case and the clock used
// to stamp FetchedAt on successful responses. Injecting the clock keeps the
// response deterministic in tests.
func NewArxivController(uc paper.UseCase, clock shared.Clock) *ArxivController {
	return &ArxivController{uc: uc, clock: clock}
}

// Fetch handles GET /api/arxiv/fetch. It takes no body and no query params;
// auth is enforced at the /api group level by the pre-existing APIToken
// middleware. On use-case error we hand off to c.Error — the ErrorEnvelope
// middleware translates *shared.HTTPError sentinels (paper.ErrUpstream*) into
// the final status and envelope.
func (ctrl *ArxivController) Fetch(c *gin.Context) {
	entries, err := ctrl.uc.Fetch(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToFetchResponse(entries, ctrl.clock.Now())))
}
