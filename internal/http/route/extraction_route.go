package route

import (
	extractionctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/extraction"
)

func ExtractionRouter(d Deps) {
	ctrl := extractionctrl.NewExtractionController(d.Extraction.UseCase)

	g := d.Group.Group("/extractions")
	g.POST("", ctrl.Submit)
	g.GET("/:id", ctrl.Get)
}
