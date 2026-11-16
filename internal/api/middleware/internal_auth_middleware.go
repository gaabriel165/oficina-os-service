package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

const InternalSecretHeader = "X-Internal-Secret"

func InternalAuth(internalSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if internalSecret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "internal authentication not configured"})
			return
		}

		provided := c.GetHeader(InternalSecretHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(internalSecret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid internal secret"})
			return
		}

		c.Next()
	}
}
