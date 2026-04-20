package route

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/paper"
	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

// ArxivConfig is the feature-scoped sub-bundle passed through route.Deps to
// wire the arXiv fetch endpoint. Bootstrap assembles it once at startup;
// ArxivRouter reads it to construct the use case and controller locally.
type ArxivConfig struct {
	Fetcher paper.Fetcher
	Query   paper.Query
}

// Deps are the shared dependencies passed to every per-resource router.
// Per-resource routers construct their own repo → usecase → controller chains from these.
type Deps struct {
	Group  *gin.RouterGroup
	DB     *gorm.DB
	Logger shared.Logger
	Clock  shared.Clock
	Arxiv  ArxivConfig
}

func Setup(d Deps) {
	HealthRouter(d)
	SourceRouter(d)
	ArxivRouter(d)
}
