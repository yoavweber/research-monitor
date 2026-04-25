package route

import (
	"github.com/yoavweber/research-monitor/backend/internal/application"
	sourcerepo "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/source"
	"github.com/yoavweber/research-monitor/backend/internal/http/controller"
)

func SourceRouter(d Deps) {
	repo := sourcerepo.NewRepository(d.DB)
	uc := application.NewSourceUseCase(repo, d.Clock)
	ctrl := controller.NewSourceController(uc)

	g := d.Group.Group("/sources")
	g.POST("", ctrl.Create)
	g.GET("", ctrl.List)
	g.GET("/:id", ctrl.Get)
	g.PATCH("/:id", ctrl.Update)
	g.DELETE("/:id", ctrl.Delete)
}
