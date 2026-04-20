package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/defi-monitor-backend/internal/domain/shared"
)

func Logger(log shared.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.InfoContext(c.Request.Context(), "http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString(RequestIDKey),
		)
	}
}
