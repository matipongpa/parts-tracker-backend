package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"partlist/internal/assistant"
	"partlist/internal/bomconverter"
	"partlist/internal/identity"
	"partlist/internal/parts"
	"partlist/internal/partstools"
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
	assistantSvc, err := newAssistant(cfg, partsSvc)
	if err != nil {
		return err
	}
	cookies := identity.CookieConfig{Secure: cfg.CookieSecure, SameSite: cfg.CookieSameSite}
	router := buildRouter(routerDeps{
		auth:        authSvc,
		cookies:     cookies,
		corsOrigins: cfg.CORSAllowedOrigins,
		parts:       partsSvc,
		bom:         bomconverter.New(partsSvc, conn),
		assistant:   assistantSvc,
		history:     assistant.NewHistory(conn, assistant.DefaultHistoryLimit),
	})
	if err := router.Run(":" + cfg.Port); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// newAssistant returns nil when no API key is configured, which leaves POST /api/ask unmounted.
func newAssistant(cfg config.Config, partsSvc parts.Service) (assistant.Service, error) {
	if cfg.AI.APIKey == "" {
		log.Printf("assistant disabled: no API key set for provider %q", cfg.AI.Provider)
		return nil, nil
	}
	log.Printf("assistant enabled: provider %q", cfg.AI.Provider)
	return assistant.New(partstools.New(partsSvc), assistant.Config{
		Provider: cfg.AI.Provider, APIKey: cfg.AI.APIKey, BaseURL: cfg.AI.BaseURL, Model: cfg.AI.Model,
	})
}

// routerDeps groups what buildRouter needs. assistant may be nil.
type routerDeps struct {
	auth        identity.Service
	cookies     identity.CookieConfig
	corsOrigins []string
	parts       parts.Service
	bom         bomconverter.Service
	assistant   assistant.Service
	history     assistant.History
}

// buildRouter mounts every module. deps.assistant may be nil, in which case /api/ask does not exist.
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
	username := func(c *gin.Context) string { return c.GetString(identity.UsernameContextKey) }
	bomconverter.RegisterRoutes(protected, deps.bom, username)
	assistant.RegisterHistoryRoutes(protected, deps.history, username)
	if deps.assistant != nil {
		assistant.RegisterAskRoutes(protected, deps.assistant, deps.history, username)
	}

	return router
}
