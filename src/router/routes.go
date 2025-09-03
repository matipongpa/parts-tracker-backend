package routes

import (
	middlewareAuth "partlist/src/app/auth/middleware"
	"partlist/src/handler"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(r *gin.Engine) {
	basePath := "/api"

	public := r.Group(basePath)
	public.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	public.POST("/login", handler.Login)
	public.POST("/register", handler.Register)

	protected := r.Group(basePath)
	protected.Use(middlewareAuth.JWTMiddleware())

	protected.GET("/part/:id", handler.GetPartById)
	protected.GET("/part", handler.GetPart)

	protected.PUT("/part/:id", handler.PutUpdatePart)

	protected.POST("/part", handler.PostCreatePart)

	protected.DELETE("/part/:id", handler.DeletePart)
}
