package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
	"github.com/yoavweber/defi-monitor-backend/internal/http/common"
)

func Recovery(log shared.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorContext(c.Request.Context(), "panic recovered", "panic", fmt.Sprintf("%v", r))
				c.AbortWithStatusJSON(http.StatusInternalServerError, common.Err(http.StatusInternalServerError, "internal server error"))
			}
		}()
		c.Next()
	}
}
