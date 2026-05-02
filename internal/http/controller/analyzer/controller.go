// Package analyzer is the HTTP controller for the /api/analyses endpoints.
// It delegates to analyzer.UseCase and never performs status mapping itself
// — the ErrorEnvelope middleware translates the wrapped *shared.HTTPError
// sentinels declared in domain/analyzer onto the wire, including the
// machine-readable details.reason discriminator for the two 502 modes.
package analyzer

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	domain "github.com/yoavweber/research-monitor/backend/internal/domain/analyzer"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// Controller is the HTTP handler for the analyzer endpoints.
type Controller struct {
	useCase domain.UseCase
}

// NewController wires the controller to its use case.
func NewController(uc domain.UseCase) *Controller {
	return &Controller{useCase: uc}
}

// Submit handles POST /api/analyses. Bind failures (missing or empty
// extraction_id, malformed JSON) wrap ErrInvalidRequest so ErrorEnvelope
// renders the 400; every other error is forwarded as-is for the middleware
// to translate.
//
// @Summary      Submit an analysis
// @Tags         Analyses
// @Accept       json
// @Produce      json
// @Param        body  body      SubmitAnalysisRequest  true  "Analysis request"
// @Success      200   {object}  AnalysisEnvelope       "Analysis persisted"
// @Failure      400   {object}  common.ErrorEnvelope   "Invalid request body"
// @Failure      401   {object}  common.ErrorEnvelope   "Missing or invalid API token"
// @Failure      404   {object}  common.ErrorEnvelope   "Extraction not found"
// @Failure      409   {object}  common.ErrorEnvelope   "Extraction not in done status"
// @Failure      500   {object}  common.ErrorEnvelope   "Analysis storage unavailable"
// @Failure      502   {object}  common.ErrorEnvelope   "LLM upstream failed or returned malformed response"
// @Security     APIToken
// @Router       /analyses [post]
func (ctrl *Controller) Submit(c *gin.Context) {
	var body SubmitAnalysisRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		_ = c.Error(fmt.Errorf("%w: %v", domain.ErrInvalidRequest, err))
		return
	}

	a, err := ctrl.useCase.Analyze(c.Request.Context(), body.ExtractionID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToAnalysisResponse(*a)))
}

// Get handles GET /api/analyses/:extraction_id. Read-only retrieval; never
// invokes the LLM.
//
// @Summary      Get an analysis by extraction id
// @Tags         Analyses
// @Produce      json
// @Param        extraction_id  path      string                 true  "Extraction id"
// @Success      200            {object}  AnalysisEnvelope       "Analysis found"
// @Failure      401            {object}  common.ErrorEnvelope   "Missing or invalid API token"
// @Failure      404            {object}  common.ErrorEnvelope   "Analysis not found"
// @Failure      500            {object}  common.ErrorEnvelope   "Analysis storage unavailable"
// @Security     APIToken
// @Router       /analyses/{extraction_id} [get]
func (ctrl *Controller) Get(c *gin.Context) {
	id := c.Param("extraction_id")
	a, err := ctrl.useCase.Get(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToAnalysisResponse(*a)))
}
