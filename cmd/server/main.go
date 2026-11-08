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
		RefreshTTL: cfg.RefreshTTL,
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
	router := buildRouter(routerDeps{
		auth:        authSvc,
		cookies:     identity.CookieConfig{Secure: cfg.CookieSecure, SameSite: cfg.CookieSameSite},
		corsOrigins: cfg.CORSAllowedOrigins,
		parts:       partsSvc,
		bom:         bomconverter.New(partsSvc),
	})
	if err := router.Run(":" + cfg.Port); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// routerDeps groups what buildRouter needs.
type routerDeps struct {
	auth        identity.Service
	cookies     identity.CookieConfig
	corsOrigins []string
	parts       parts.Service
	bom         bomconverter.Service
}

func buildRouter(deps routerDeps) *gin.Engine {
	router := gin.Default()
	router.Use(httpx.NewCORS(deps.corsOrigins))

	public := router.Group(apiBasePath)
	public.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})
	identity.RegisterRoutes(public, deps.auth, deps.cookies)

	protected := router.Group(apiBasePath)
	protected.Use(identity.Middleware(deps.auth))
	parts.RegisterRoutes(protected, deps.parts)
	bomconverter.RegisterRoutes(protected, deps.bom)

	return router
}
