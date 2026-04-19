package route

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/defi-monitor-backend/internal/interface/http/common"
)

func HealthRouter(d Deps) {
	d.Group.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, common.Data(gin.H{"status": "ok"}))
	})
}
