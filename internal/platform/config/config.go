package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/joho/godotenv"
)

const (
	defaultPort              string        = "8080"
	defaultTokenTTL          time.Duration = 15 * time.Minute
	defaultRefreshTTL        time.Duration = 7 * 24 * time.Hour
	defaultCORSOrigin        string        = "http://localhost:4200"
	defaultPartsCacheEntries int           = 1000
	defaultPartsCacheTTL     time.Duration = 60 * time.Second
)

// Config is loaded once in cmd/server and passed down to modules.
type Config struct {
	Port       string
	JWTSecret  string
	DBURL      string
	BcryptCost int
	// TokenTTL is the access token lifetime (TOKEN_TTL). The UI keeps it in memory and refreshes it.
	TokenTTL time.Duration
	// RefreshTTL is the refresh token / cookie lifetime (REFRESH_TOKEN_TTL).
	RefreshTTL time.Duration
	// CookieSecure marks the refresh cookie HTTPS-only (COOKIE_SECURE, default true).
	CookieSecure bool
	// CookieSameSite is the refresh cookie's SameSite mode (COOKIE_SAMESITE: lax, strict or none).
	CookieSameSite http.SameSite
	// CORSAllowedOrigins are the UI origins allowed to call the API with credentials (CORS_ALLOWED_ORIGINS).
	CORSAllowedOrigins []string
	// InviteCode is the shared secret required by POST /api/register (REGISTER_INVITE_CODE).
	// Empty closes registration.
	InviteCode string
	// RunMigrations applies pending SQL migrations at startup (RUN_MIGRATIONS=true).
	RunMigrations bool
	// PartsCacheMaxEntries <= 0 disables the parts search cache.
	PartsCacheMaxEntries int
	PartsCacheTTL        time.Duration
}

// Load reads an optional .env file, then validates the process environment.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}
	return fromEnv(os.Getenv)
}

func fromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		Port:                 getenv("PORT"),
		JWTSecret:            getenv("JWT_SECRET"),
		DBURL:                getenv("DB_URL"),
		InviteCode:           strings.TrimSpace(getenv("REGISTER_INVITE_CODE")),
		BcryptCost:           bcrypt.DefaultCost,
		TokenTTL:             defaultTokenTTL,
		RefreshTTL:           defaultRefreshTTL,
		CookieSecure:         true,
		CookieSameSite:       http.SameSiteLaxMode,
		PartsCacheMaxEntries: defaultPartsCacheEntries,
		PartsCacheTTL:        defaultPartsCacheTTL,
	}

	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("config: JWT_SECRET is required")
	}
	if cfg.DBURL == "" {
		return Config{}, errors.New("config: DB_URL is required")
	}

	if raw := getenv("RUN_MIGRATIONS"); raw != "" {
		run, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: RUN_MIGRATIONS must be a boolean: %w", err)
		}
		cfg.RunMigrations = run
	}

	if raw := getenv("PARTS_CACHE_MAX_ENTRIES"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: PARTS_CACHE_MAX_ENTRIES must be an integer: %w", err)
		}
		cfg.PartsCacheMaxEntries = n
	}
	if raw := getenv("PARTS_CACHE_TTL"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: PARTS_CACHE_TTL must be a duration like 60s: %w", err)
		}
		cfg.PartsCacheTTL = ttl
	}

	if err := authFromEnv(getenv, &cfg); err != nil {
		return Config{}, err
	}

	if raw := getenv("BCRYPT_COST"); raw != "" {
		cost, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config: BCRYPT_COST must be an integer: %w", err)
		}
		if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
			return Config{}, fmt.Errorf("config: BCRYPT_COST must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
		}
		cfg.BcryptCost = cost
	}

	return cfg, nil
}

// authFromEnv reads token lifetimes, the refresh cookie attributes and the CORS allow-list.
func authFromEnv(getenv func(string) string, cfg *Config) error {
	for _, ttl := range []struct {
		key string
		dst *time.Duration
	}{{"TOKEN_TTL", &cfg.TokenTTL}, {"REFRESH_TOKEN_TTL", &cfg.RefreshTTL}} {
		raw := getenv(ttl.key)
		if raw == "" {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return fmt.Errorf("config: %s must be a positive duration like 15m: %q", ttl.key, raw)
		}
		*ttl.dst = d
	}

	if raw := getenv("COOKIE_SECURE"); raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("config: COOKIE_SECURE must be a boolean: %w", err)
		}
		cfg.CookieSecure = secure
	}

	if raw := strings.ToLower(strings.TrimSpace(getenv("COOKIE_SAMESITE"))); raw != "" {
		switch raw {
		case "lax":
			cfg.CookieSameSite = http.SameSiteLaxMode
		case "strict":
			cfg.CookieSameSite = http.SameSiteStrictMode
		case "none":
			cfg.CookieSameSite = http.SameSiteNoneMode
		default:
			return fmt.Errorf("config: COOKIE_SAMESITE must be lax, strict or none, got %q", raw)
		}
	}
	// Browsers drop a SameSite=None cookie that is not Secure, which would silently break refresh.
	if cfg.CookieSameSite == http.SameSiteNoneMode && !cfg.CookieSecure {
		return errors.New("config: COOKIE_SAMESITE=none requires COOKIE_SECURE=true")
	}

	origins, err := corsOriginsFromEnv(getenv("CORS_ALLOWED_ORIGINS"))
	if err != nil {
		return err
	}
	cfg.CORSAllowedOrigins = origins
	return nil
}

// corsOriginsFromEnv parses a comma-separated list of origins. Credentials are allowed, so "*" is
// not: each entry must be an exact scheme://host[:port].
func corsOriginsFromEnv(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{defaultCORSOrigin}, nil
	}
	var origins []string
	for _, part := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(part)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			return nil, fmt.Errorf("config: CORS_ALLOWED_ORIGINS entry %q must look like https://host[:port]", origin)
		}
		origins = append(origins, origin)
	}
	if len(origins) == 0 {
		return nil, errors.New("config: CORS_ALLOWED_ORIGINS has no origins")
	}
	return origins, nil
}
