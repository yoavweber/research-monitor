package route

import (
	extractionctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/extraction"
)

// ExtractionRouter wires the POST /api/extractions and GET /api/extractions/:id
// handlers under the authenticated /api group. Auth is supplied by the
// APIToken middleware already mounted on the /api group; we don't reauthor it.
func ExtractionRouter(d Deps) {
	ctrl := extractionctrl.NewExtractionController(d.Extraction.UseCase)

	g := d.Group.Group("/extractions")
	g.POST("", ctrl.Submit)
	g.GET("/:id", ctrl.Get)
}
