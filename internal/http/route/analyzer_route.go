package route

import (
	analyzerctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/analyzer"
)

// AnalyzerRouter registers the /analyses endpoints under the /api group so
// they inherit the existing X-API-Token middleware and ErrorEnvelope
// translation without introducing a new auth surface.
func AnalyzerRouter(d Deps) {
	ctrl := analyzerctrl.NewController(d.Analyzer.UseCase)

	g := d.Group.Group("/analyses")
	g.POST("", ctrl.Submit)
	g.GET("/:extraction_id", ctrl.Get)
}
