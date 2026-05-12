package paper

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/application/pdfdownload"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// PDFDownloadReader is the consumer-defined read surface for the
// per-job status and SSE endpoints. The concrete *pdfdownload.Registry
// satisfies it implicitly.
type PDFDownloadReader interface {
	Snapshot(ctx context.Context, id pdfdownload.JobID) (pdfdownload.JobSnapshot, error)
	Subscribe(ctx context.Context, id pdfdownload.JobID) (backlog []pdfdownload.Event, live <-chan pdfdownload.Event, err error)
}

// PDFDownloadController hosts the per-job read endpoints (status JSON +
// SSE stream) for PDF-download jobs scheduled by /api/arxiv/fetch.
type PDFDownloadController struct {
	reader PDFDownloadReader
}

func NewPDFDownloadController(reader PDFDownloadReader) *PDFDownloadController {
	return &PDFDownloadController{reader: reader}
}

// Status godoc
//
// @Summary      Get the status of a PDF-download job
// @Description  Returns a snapshot of a PDF-download job scheduled by
// @Description  /api/arxiv/fetch, including per-entry results and totals.
// @Tags         PDFDownload
// @Produce      json
// @Param        job_id  path      string  true  "Download job id (UUIDv4)"
// @Success      200     {object}  JobStatusEnvelope     "Job status snapshot"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404     {object}  common.ErrorEnvelope  "Job unknown or expired"
// @Security     APIToken
// @Router       /arxiv/downloads/{job_id} [get]
func (ctrl *PDFDownloadController) Status(c *gin.Context) {
	id := pdfdownload.JobID(c.Param("job_id"))

	snap, err := ctrl.reader.Snapshot(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, common.Data(ToDownloadJobSnapshotDTO(snap)))
}

// Stream godoc
//
// Stream replays the buffered backlog then forwards live events. The
// terminal download.summary frame closes the response. An unknown or
// evicted job returns 404 before any SSE header is written, so the
// client receives a JSON envelope rather than an empty stream. A live
// channel that closes without a Summary (slow-consumer drop or
// registry shutdown) ends the response with a final `event: error`
// frame; client disconnect via request-ctx cancellation returns
// without affecting the underlying job.
//
// @Summary      Stream PDF-download events
// @Description  Opens a Server-Sent Events stream for a PDF-download
// @Description  job: replays buffered events first, then forwards live
// @Description  progress events and a terminal summary frame.
// @Tags         PDFDownload
// @Produce      text/event-stream
// @Param        job_id  path      string  true  "Download job id (UUIDv4)"
// @Success      200     {string}  string                "SSE stream of download.progress events followed by a terminal download.summary"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404     {object}  common.ErrorEnvelope  "Job unknown or expired"
// @Security     APIToken
// @Router       /arxiv/downloads/{job_id}/stream [get]
func (ctrl *PDFDownloadController) Stream(c *gin.Context) {
	id := pdfdownload.JobID(c.Param("job_id"))

	backlog, live, err := ctrl.reader.Subscribe(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	for _, ev := range backlog {
		if writeDownloadEvent(c, ev) {
			return
		}
	}

	ctxDone := c.Request.Context().Done()
	for {
		select {
		case <-ctxDone:
			return
		case ev, ok := <-live:
			if !ok {
				// Slow-consumer drop or registry shutdown — surface a
				// stable terminal frame so clients fall back to /status.
				c.SSEvent("error", map[string]string{"reason": "stream_closed"})
				c.Writer.Flush()
				return
			}
			if writeDownloadEvent(c, ev) {
				return
			}
		}
	}
}

// writeDownloadEvent serializes an Event into the matching SSE frame
// and returns true after emitting a terminal Summary.
func writeDownloadEvent(c *gin.Context, ev pdfdownload.Event) (terminal bool) {
	switch {
	case ev.Summary != nil:
		s := ev.Summary
		c.SSEvent("download.summary", DownloadSummaryEventDTO{
			JobID:     string(s.JobID),
			Total:     s.Total,
			Succeeded: s.Succeeded,
			Failed:    s.Failed,
		})
		c.Writer.Flush()
		return true
	case ev.Progress != nil:
		c.SSEvent("download.progress", DownloadProgressEventDTO(ToDownloadEntryResultDTO(*ev.Progress)))
		c.Writer.Flush()
		return false
	default:
		return false
	}
}
