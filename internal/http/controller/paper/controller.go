// Package paper hosts the HTTP handler and wire DTOs for the
// GET /api/papers endpoints (single-paper lookup by composite key and full
// catalogue listing). The domain layer (internal/domain/paper) carries no
// response shapes — this package owns the JSON contract and the mapping from
// paper.Entry into it.
package paper

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// PaperController is the HTTP handler for the /api/papers endpoints. The repo
// is held directly (no application-layer wrapper) because the read paths are
// thin pass-throughs — there is no orchestration to justify another layer.
// Errors are never mapped to status codes here; the ErrorEnvelope middleware
// owns that translation off the *shared.HTTPError sentinels declared in the
// paper domain package.
type PaperController struct {
	repo  paper.Repository
	clock shared.Clock
}

// NewPaperController returns the controller with its repository and clock
// dependencies. The clock is currently unused but kept for symmetry with the
// arxiv controller — adding ceremony to remove it costs more than carrying it.
func NewPaperController(repo paper.Repository, clock shared.Clock) *PaperController {
	return &PaperController{repo: repo, clock: clock}
}

// Get handles GET /api/papers/:source/:source_id. Path params are pulled via
// c.Param; the repo translates a missing row into paper.ErrNotFound and any
// other storage failure into paper.ErrCatalogueUnavailable. On either error we
// hand off to c.Error so ErrorEnvelope renders the final status and envelope.
func (ctrl *PaperController) Get(c *gin.Context) {
	source := c.Param("source")
	sourceID := c.Param("source_id")

	entry, err := ctrl.repo.FindByKey(c.Request.Context(), source, sourceID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToPaperResponse(*entry)))
}

// List handles GET /api/papers. The repo is the source of ordering — the
// controller returns whatever order the repo emits without re-sorting.
func (ctrl *PaperController) List(c *gin.Context) {
	entries, err := ctrl.repo.List(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToPaperListResponse(entries)))
}
