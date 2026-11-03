package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/joho/godotenv"
)

const (
	defaultPort     string        = "8080"
	defaultTokenTTL time.Duration = time.Hour
)

// Config is loaded once in cmd/server and passed down to modules.
type Config struct {
	Port       string
	JWTSecret  string
	DBURL      string
	BcryptCost int
	TokenTTL   time.Duration
	// RunMigrations applies pending SQL migrations at startup (RUN_MIGRATIONS=true).
	RunMigrations bool
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
		Port:       getenv("PORT"),
		JWTSecret:  getenv("JWT_SECRET"),
		DBURL:      getenv("DB_URL"),
		BcryptCost: bcrypt.DefaultCost,
		TokenTTL:   defaultTokenTTL,
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
