package app

import (
	"log"
	routes "partlist/src/router"

	db "partlist/src/db"

	"github.com/gin-gonic/gin"
)

func Run() {
	r := gin.Default()
	routes.RegisterRoutes(r)

	if err := db.InitDB(); err != nil {
		log.Fatal("failed to connect database: ", err)
	}

	r.Run() // listen and serve on 0.0.0.0:8080
}
