package middlewareAuth

import (
	"net/http"
	jwtAuth "partlist/src/app/auth/jwt"
	"strings"

	"github.com/gin-gonic/gin"
)

func JWTMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("Authorization")
		if tokenStr == "" || !strings.HasPrefix(tokenStr, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid token"})
			return
		}
		tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

		claims, err := jwtAuth.VerifyToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Set(string(jwtAuth.UserNameKey), claims.UserName)
		c.Next()
	}
}
