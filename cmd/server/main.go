package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"partlist/internal/bomconverter"
	"partlist/internal/identity"
	"partlist/internal/parts"
	"partlist/internal/platform/config"
	"partlist/internal/platform/db"
	"partlist/internal/platform/httpx"
	"partlist/internal/platform/migrate"
	"partlist/migrations"
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
	if cfg.RunMigrations {
		if err := migrate.Up(conn, migrations.FS); err != nil {
			return err
		}
	}

	authSvc, err := identity.New(conn, identity.Config{
		JWTSecret:  cfg.JWTSecret,
		TokenTTL:   cfg.TokenTTL,
		BcryptCost: cfg.BcryptCost,
		InviteCode: cfg.InviteCode,
	})
	if err != nil {
		return err
	}
	if cfg.InviteCode == "" {
		log.Print("registration closed: REGISTER_INVITE_CODE is not set")
	}

	partsSvc := parts.New(conn, parts.Options{
		CacheMaxEntries: cfg.PartsCacheMaxEntries,
		CacheTTL:        cfg.PartsCacheTTL,
	})
	router := buildRouter(authSvc, partsSvc, bomconverter.New(partsSvc))
	if err := router.Run(":" + cfg.Port); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func buildRouter(authSvc identity.Service, partsSvc parts.Service, bomSvc bomconverter.Service) *gin.Engine {
	router := gin.Default()
	router.Use(httpx.NewCORS())

	public := router.Group(apiBasePath)
	public.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	identity.RegisterRoutes(public, authSvc)

	protected := router.Group(apiBasePath)
	protected.Use(identity.Middleware(authSvc))
	parts.RegisterRoutes(protected, partsSvc)
	bomconverter.RegisterRoutes(protected, bomSvc)

	return router
}
