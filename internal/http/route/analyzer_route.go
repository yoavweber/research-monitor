package route

import (
	analyzerctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/analyzer"
)

func AnalyzerRouter(d Deps) {
	ctrl := analyzerctrl.NewController(d.Analyzer.UseCase)

	g := d.Group.Group("/analyses")
	g.POST("", ctrl.Submit)
	g.GET("/:extraction_id", ctrl.Get)
}
