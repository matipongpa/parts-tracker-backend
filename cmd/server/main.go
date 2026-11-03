package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"partlist/internal/identity"
	"partlist/internal/platform/config"
	"partlist/internal/platform/db"
	"partlist/internal/platform/httpx"

	// Legacy part handlers; replaced by internal/parts in step 2 of docs/design/modular-monolith.md.
	legacyhandler "partlist/src/handler"

	legacydb "partlist/src/db"
)

const apiBasePath string = "/api"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	conn, err := db.Open(cfg.DBURL)
	if err != nil {
		return err
	}
	// Bridge for the legacy part handlers that still read the package-level handle.
	legacydb.SetDB(conn)

	authSvc, err := identity.New(conn, identity.Config{
		JWTSecret:  cfg.JWTSecret,
		TokenTTL:   cfg.TokenTTL,
		BcryptCost: cfg.BcryptCost,
	})
	if err != nil {
		return err
	}

	router := buildRouter(authSvc)
	if err := router.Run(":" + cfg.Port); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func buildRouter(authSvc identity.Service) *gin.Engine {
	router := gin.Default()
	router.Use(httpx.NewCORS())

	public := router.Group(apiBasePath)
	public.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	identity.RegisterRoutes(public, authSvc)

	protected := router.Group(apiBasePath)
	protected.Use(identity.Middleware(authSvc))
	protected.GET("/part/:id", legacyhandler.GetPartById)
	protected.GET("/part", legacyhandler.GetPart)
	protected.PUT("/part/:id", legacyhandler.PutUpdatePart)
	protected.POST("/part", legacyhandler.PostCreatePart)
	protected.DELETE("/part/:id", legacyhandler.DeletePart)

	return router
}
