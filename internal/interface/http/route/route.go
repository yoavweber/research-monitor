package route

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

// Deps are the shared dependencies passed to every per-resource router.
// Per-resource routers construct their own repo → usecase → controller chains from these.
type Deps struct {
	Group  *gin.RouterGroup
	DB     *gorm.DB
	Logger shared.Logger
	Clock  shared.Clock
}

func Setup(d Deps) {
	HealthRouter(d)
	// SourceRouter(d) // uncomment in Task 18
}
