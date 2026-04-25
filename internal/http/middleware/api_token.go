package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/yoavweber/research-monitor/backend/internal/http/common"
)

const APITokenHeader = "X-API-Token"

func APIToken(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		got := c.GetHeader(APITokenHeader)
		if got == "" || got != expected {
			c.AbortWithStatusJSON(http.StatusUnauthorized, common.Err(http.StatusUnauthorized, "invalid or missing api token"))
			return
		}
		c.Next()
	}
}
