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

	r.GET("/part/:id", handler.GetPartById)
	r.GET("/part", handler.GetPart)

	r.PUT("/part/:id", handler.PutUpdatePart)

	r.POST("/part", handler.PostCreatePart)

	r.DELETE("/part/:id", handler.DeletePart)
}
