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
// The controller exposes the JSON status handler (Status) and the
// Server-Sent Events stream handler (Stream).
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

// Stream godoc
//
// Stream handles GET /api/arxiv/downloads/{job_id}/stream. It opens a
// Server-Sent Events response that first replays the buffered backlog
// recorded for the job, then forwards live events from the registry's
// per-subscriber channel. The handler emits one `download.progress`
// frame per per-entry result and a single terminal `download.summary`
// frame carrying the job totals, after which the response is closed.
//
// Errors:
//   - paper.ErrDownloadJobUnknown returns HTTP 404 via the standard
//     ErrorEnvelope middleware before any SSE frame is written, so a
//     client whose job was unknown or evicted receives a JSON error
//     envelope rather than an empty stream.
//   - A live channel closing without a Summary frame (slow-subscriber
//     drop or registry shutdown) terminates the response with a final
//     `event: error` frame and a small JSON reason payload.
//   - A client disconnect (request context cancellation) returns
//     immediately without affecting the underlying job or other
//     subscribers.
//
// @Summary      Stream PDF-download events
// @Description  Opens a Server-Sent Events stream for a PDF-download
// @Description  job: replays buffered events first, then forwards live
// @Description  progress events and a terminal summary frame.
// @Tags         PDFDownload
// @Produce      text/event-stream
// @Param        job_id  path      string                "Download job id (UUIDv4)"
// @Success      200     {string}  string                "SSE stream of download.progress events followed by a terminal download.summary"
// @Failure      401     {object}  common.ErrorEnvelope  "Missing or invalid API token"
// @Failure      404     {object}  common.ErrorEnvelope  "Job unknown or expired"
// @Security     APIToken
// @Router       /arxiv/downloads/{job_id}/stream [get]
func (ctrl *PDFDownloadController) Stream(c *gin.Context) {
	id := paper.DownloadJobID(c.Param("job_id"))

	backlog, live, err := ctrl.reader.SubscribePDFDownloadJob(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, paper.ErrDownloadJobUnknown) {
			// Surface as the canonical 404 envelope BEFORE writing any
			// SSE frame so a client whose job was unknown or evicted
			// receives a JSON error rather than an empty stream.
			_ = c.Error(shared.NewHTTPError(http.StatusNotFound, "download job unknown", err))
			return
		}
		_ = c.Error(err)
		return
	}

	// Headers are set only on the success path so the 404 envelope
	// remains JSON. X-Accel-Buffering suppresses nginx buffering as a
	// defensive default for production deployments behind nginx.
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	// Replay backlog first. If the backlog already contains the
	// terminal Summary (job completed at the moment of Subscribe), we
	// must NOT enter the live loop — the live channel may be nil or a
	// pre-closed channel.
	for _, ev := range backlog {
		if writeDownloadEvent(c, ev) {
			return
		}
	}

	if live == nil {
		return
	}

	ctxDone := c.Request.Context().Done()
	for {
		select {
		case <-ctxDone:
			// Client disconnect: stop writing. The underlying job is
			// owned by the registry's worker and is not affected.
			return
		case ev, ok := <-live:
			if !ok {
				// Live channel closed without a Summary frame — the
				// subscriber was dropped (slow-consumer policy) or the
				// registry is shutting down. Emit a stable error frame
				// so the client can fall back to the status endpoint.
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

// writeDownloadEvent serializes a paper.DownloadEvent into the matching
// SSE frame. Returns true when the caller must stop streaming (the
// terminal Summary frame was just emitted).
func writeDownloadEvent(c *gin.Context, ev paper.DownloadEvent) (terminal bool) {
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
		// Neither Progress nor Summary set — invariant violation by the
		// producer; skip rather than crash the stream.
		return false
	}
}
