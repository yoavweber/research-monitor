package route

import (
	paperctrl "github.com/yoavweber/research-monitor/backend/internal/http/controller/paper"
)

// PaperRouter wires the source-neutral /api/papers read endpoints. The
// controller takes the persisted paper.Repository directly — there is no
// orchestration to justify an application-layer wrapper for these read
// paths. Auth is supplied by the APIToken middleware already mounted on
// the /api group.
func PaperRouter(d Deps) {
	ctrl := paperctrl.NewPaperController(d.Paper.Repo)

	g := d.Group.Group("/papers")
	g.GET("", ctrl.List)
	g.GET("/:source/:source_id", ctrl.Get)
}
