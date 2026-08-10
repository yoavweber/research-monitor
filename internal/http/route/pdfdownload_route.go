package route

import (
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
)

// PDFDownloadRouter wires the per-job read endpoints for PDF download
// jobs scheduled by /api/arxiv/fetch. The endpoints sit under
// /api/arxiv/downloads/:job_id so they inherit the /api group's
// APIToken middleware. The Reader port is shared with the arxiv use
// case's PDFScheduler via the same in-memory registry instance, which
// bootstrap constructs once.
func PDFDownloadRouter(d Deps) {
	ctrl := paperctrl.NewPDFDownloadController(d.Download.Reader)

	g := d.Group.Group("/arxiv/downloads")
	g.GET("/:job_id", ctrl.Status)
	g.GET("/:job_id/stream", ctrl.Stream)
}
