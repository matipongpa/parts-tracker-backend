package httpx

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// NewCORS returns the CORS middleware. It does not handle authentication.
func NewCORS() gin.HandlerFunc {
	return cors.New(cors.Config{
		// In production these should be the frontend domain(s).
		AllowOrigins: []string{
			"http://localhost:4200",
			"https://your-frontend.com",
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           4 * time.Hour,
	})
}
