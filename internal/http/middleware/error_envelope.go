package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/domain/shared"
	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

// ErrorEnvelope converts ctx.Error(err) calls into a JSON error envelope.
// If err is *shared.HTTPError, use its Code + Message. Otherwise 500.
func ErrorEnvelope() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		last := c.Errors.Last().Err
		if he := shared.AsHTTPError(last); he != nil {
			c.AbortWithStatusJSON(he.Code, common.Err(he.Code, he.Message))
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, common.Err(http.StatusInternalServerError, "internal server error"))
	}
}
