package routes

import (
	"partlist/src/handler"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.GET("/capacitor/:id", handler.GetCapacitorById)
	r.GET("/capacitor", handler.GetCapacitor)

	r.PUT("/capacitor/:id", handler.PutUpdateCapacitor)

	r.POST("/capacitor", handler.PostCreateCapacitor)
}
