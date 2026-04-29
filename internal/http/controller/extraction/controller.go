package extraction

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/extraction"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// ExtractionController is the HTTP handler for the /api/extractions endpoints.
// It delegates to extraction.UseCase, never performs status-mapping on errors
// itself (the ErrorEnvelope middleware owns that off the *shared.HTTPError
// sentinels declared in the extraction domain package), and owns the wire
// shape via requests.go / responses.go.
type ExtractionController struct {
	useCase extraction.UseCase
}

// NewExtractionController wires the controller to its use case. Returning a
// concrete pointer (not an interface) mirrors the paper / arxiv controllers.
func NewExtractionController(uc extraction.UseCase) *ExtractionController {
	return &ExtractionController{useCase: uc}
}

// Submit handles POST /api/extractions. The handler binds the request body,
// translates it to the domain RequestPayload (so the use case's Validate
// runs through the same path non-HTTP entrypoints use), invokes the use
// case, and returns 202 with the assigned id and initial status. A failing
// JSON bind is wrapped with ErrInvalidRequest so the ErrorEnvelope
// middleware renders the standard 400 envelope.
//
// @Summary      Submit an extraction
// @Tags         Extractions
// @Accept       json
// @Produce      json
// @Param        body  body      SubmitExtractionRequest        true  "Extraction request"
// @Success      202   {object}  ExtractionStatusEnvelope       "Extraction enqueued"
// @Failure      400   {object}  common.ErrorEnvelope           "Invalid request body"
// @Failure      401   {object}  common.ErrorEnvelope           "Missing or invalid API token"
// @Failure      500   {object}  common.ErrorEnvelope           "Catalogue unavailable"
// @Security     APIToken
// @Router       /extractions [post]
func (ctrl *ExtractionController) Submit(c *gin.Context) {
	var body SubmitExtractionRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		// Wrap with the domain ErrInvalidRequest sentinel so ErrorEnvelope
		// renders 400 with the standard envelope. The bind error itself is
		// preserved as the wrapped cause for log correlation.
		_ = c.Error(fmt.Errorf("%w: %v", extraction.ErrInvalidRequest, err))
		return
	}
	payload := extraction.RequestPayload{
		SourceType: body.SourceType,
		SourceID:   body.SourceID,
		PDFPath:    body.PDFPath,
	}
	result, err := ctrl.useCase.Submit(c.Request.Context(), payload)
	if err != nil {
		_ = c.Error(err)
		return
	}
	response := ExtractionStatusResponse{
		ID:         result.ID,
		SourceType: payload.SourceType,
		SourceID:   payload.SourceID,
		Status:     string(result.Status),
	}
	c.JSON(http.StatusAccepted, common.Data(response))
}

// Get handles GET /api/extractions/:id. Returns the current state of the
// extraction, including the artifact when status=done or the failure
// reason+message when status=failed; pending / running responses omit both
// blocks via the ToExtractionStatusResponse omitempty rule.
//
// @Summary      Get extraction status by id
// @Tags         Extractions
// @Produce      json
// @Param        id    path      string                          true  "Extraction id"
// @Success      200   {object}  ExtractionStatusEnvelope        "Extraction state"
// @Failure      401   {object}  common.ErrorEnvelope            "Missing or invalid API token"
// @Failure      404   {object}  common.ErrorEnvelope            "Extraction not found"
// @Failure      500   {object}  common.ErrorEnvelope            "Catalogue unavailable"
// @Security     APIToken
// @Router       /extractions/{id} [get]
func (ctrl *ExtractionController) Get(c *gin.Context) {
	id := c.Param("id")
	e, err := ctrl.useCase.Get(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToExtractionStatusResponse(*e)))
}
