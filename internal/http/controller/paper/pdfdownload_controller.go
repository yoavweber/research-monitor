package paper

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/paper"
	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// PDFDownloadController hosts the per-job read endpoints for PDF download
// jobs scheduled by /api/arxiv/fetch. It depends only on the
// paper.PDFDownloadReader port; the concrete registry lives in
// application/pdfdownload and is wired in bootstrap.
//
// Today the controller exposes only the JSON status handler (Status);
// task 3.2 will add the SSE stream handler on the same struct.
type PDFDownloadController struct {
	reader paper.PDFDownloadReader
}

// NewPDFDownloadController constructs the controller from its read port.
// The constructor returns the concrete type so route wiring can attach
// per-handler methods; the type's only external dependency is the port.
func NewPDFDownloadController(reader paper.PDFDownloadReader) *PDFDownloadController {
	return &PDFDownloadController{reader: reader}
}

// Status godoc
//
// Status handles GET /api/arxiv/downloads/{job_id}. It calls
// SnapshotPDFDownloadJob and marshals the snapshot through the shared
// DTO mappers. Unknown or evicted jobs surface as
// paper.ErrDownloadJobUnknown, which the controller wraps into a
// *shared.HTTPError so the existing ErrorEnvelope middleware emits the
// canonical 404 envelope without per-handler duplication.
//
// @Summary      Get the status of a PDF-download job
// @Description  Returns a snapshot of a PDF-download job scheduled by
// @Description  /api/arxiv/fetch, including per-entry results and totals.
// @Tags         PDFDownload
// @Produce      json
// @Param        job_id  path      string                "Download job id (UUIDv4)"
// @Success      200     {object}  JobStatusEnvelope     "Job status snapshot"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404     {object}  common.ErrorEnvelope  "Job unknown or expired"
// @Security     APIToken
// @Router       /arxiv/downloads/{job_id} [get]
func (ctrl *PDFDownloadController) Status(c *gin.Context) {
	id := paper.DownloadJobID(c.Param("job_id"))

	snap, err := ctrl.reader.SnapshotPDFDownloadJob(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, paper.ErrDownloadJobUnknown) {
			// Wrap with the existing sentinel as cause so callers using
			// errors.Is can still recognize the domain error if they
			// introspect c.Errors.
			_ = c.Error(shared.NewHTTPError(http.StatusNotFound, "download job unknown", err))
			return
		}
		_ = c.Error(err)
		return
	}

	c.JSON(http.StatusOK, common.Data(ToDownloadJobSnapshotDTO(snap)))
}
