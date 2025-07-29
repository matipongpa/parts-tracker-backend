package routes

import (
	"partlist/src/handler"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	basePath := "/api"
	r.GET(basePath+"/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.GET(basePath+"/part/:id", handler.GetPartById)
	r.GET(basePath+"/part", handler.GetPart)

	r.PUT(basePath+"/part/:id", handler.PutUpdatePart)

	r.POST(basePath+"/part", handler.PostCreatePart)

	r.DELETE(basePath+"/part/:id", handler.DeletePart)
}
