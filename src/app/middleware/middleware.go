package middleware

import (
	"time" // Needed for cors.Config.MaxAge

	"github.com/gin-contrib/cors" // Import the CORS middleware package
	"github.com/gin-gonic/gin"
)

// CorsMiddleware returns a Gin middleware for handling CORS.
// This function does NOT handle authentication.
func NewCorr() gin.HandlerFunc {
	return cors.New(cors.Config{
		// IMPORTANT: Configure your allowed origins here.
		// In production, these should be your frontend domain(s).
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
