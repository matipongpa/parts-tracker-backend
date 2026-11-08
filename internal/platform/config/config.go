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
	defaultPartsCacheTTL     time.Duration = 300 * time.Second
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
	// AI configures POST /api/ask. It is enabled only when AI.APIKey is set.
	AI AIConfig
}

// AI providers selectable with AI_PROVIDER.
const (
	AIProviderAnthropic        string = "anthropic"
	AIProviderOpenAICompatible string = "openai-compatible"
)

// AIConfig selects the language model behind POST /api/ask.
type AIConfig struct {
	Provider string
	// APIKey is ANTHROPIC_API_KEY for the anthropic provider and AI_API_KEY for openai-compatible.
	// Empty leaves the assistant disabled.
	APIKey string
	// BaseURL is the API root of an openai-compatible server, e.g. https://host/api/v1.
	BaseURL string
	// Model is required for openai-compatible; empty selects the assistant's default for anthropic.
	Model string
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

	ai, err := aiFromEnv(getenv)
	if err != nil {
		return Config{}, err
	}
	cfg.AI = ai

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

// aiFromEnv reads the assistant settings. The assistant stays off unless the provider's key is set;
// once it is set, the rest of that provider's settings must be valid so a typo fails at startup.
func aiFromEnv(getenv func(string) string) (AIConfig, error) {
	provider := strings.ToLower(strings.TrimSpace(getenv("AI_PROVIDER")))
	if provider == "" {
		provider = AIProviderAnthropic
	}
	ai := AIConfig{Provider: provider, Model: strings.TrimSpace(getenv("AI_MODEL"))}

	switch provider {
	case AIProviderAnthropic:
		ai.APIKey = getenv("ANTHROPIC_API_KEY")
	case AIProviderOpenAICompatible:
		ai.APIKey = getenv("AI_API_KEY")
		ai.BaseURL = strings.TrimSpace(getenv("AI_BASE_URL"))
		if ai.APIKey == "" {
			return ai, nil
		}
		if ai.Model == "" {
			return AIConfig{}, errors.New("config: AI_MODEL is required when AI_PROVIDER is openai-compatible")
		}
		if err := validateBaseURL(ai.BaseURL); err != nil {
			return AIConfig{}, fmt.Errorf("config: AI_BASE_URL: %w", err)
		}
	default:
		return AIConfig{}, fmt.Errorf("config: AI_PROVIDER must be %q or %q, got %q", AIProviderAnthropic, AIProviderOpenAICompatible, provider)
	}
	return ai, nil
}

// validateBaseURL requires https, so the API key never travels in clear text, except to a
// loopback address (a local Ollama, for example).
func validateBaseURL(raw string) error {
	if raw == "" {
		return errors.New("is required when AI_PROVIDER is openai-compatible")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a valid URL", raw)
	}
	if u.User != nil {
		return errors.New("must not contain credentials")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return nil
		}
		return errors.New("must use https unless it points at localhost")
	}
	return fmt.Errorf("scheme must be https, got %q", u.Scheme)
}
