package middleware

import (
	"net/http"
	"strings"

	"github.com/ai-code-review/aicr/internal/auth"
	"github.com/gin-gonic/gin"
)

// JWTAuth 校验 Authorization: Bearer <token>。
func JWTAuth(mgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")
		claims, err := mgr.Verify(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已过期"})
			return
		}
		c.Set("user", claims.User)
		c.Next()
	}
}
