package app

import (
	"log"
	"partlist/src/app/middleware"
	routes "partlist/src/router"

	db "partlist/src/db"
	config "partlist/src/utilis"

	"github.com/gin-gonic/gin"
)

func Run() {
	config.Init()
	appConfig := config.GetConfig()

	r := gin.Default()
	r.Use(middleware.NewCorr())

	routes.RegisterRoutes(r)

	if err := db.InitDB(appConfig.DBUrl); err != nil {
		log.Fatal("failed to connect database: ", err)
	}

	r.Run() // listen and serve on 0.0.0.0:8080
}
