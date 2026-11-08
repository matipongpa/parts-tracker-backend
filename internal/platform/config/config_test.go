package config

import (
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestFromEnv(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		wantErr  bool
		wantPort string
		wantCost int
	}{
		{
			name:     "defaults applied",
			env:      map[string]string{"JWT_SECRET": "s", "DB_URL": "postgres://x"},
			wantPort: "8080",
			wantCost: bcrypt.DefaultCost,
		},
		{
			name:     "explicit values",
			env:      map[string]string{"JWT_SECRET": "s", "DB_URL": "postgres://x", "PORT": "9000", "BCRYPT_COST": "12"},
			wantPort: "9000",
			wantCost: 12,
		},
		{name: "missing secret", env: map[string]string{"DB_URL": "postgres://x"}, wantErr: true},
		{name: "missing db url", env: map[string]string{"JWT_SECRET": "s"}, wantErr: true},
		{
			name:    "cost not a number",
			env:     map[string]string{"JWT_SECRET": "s", "DB_URL": "postgres://x", "BCRYPT_COST": "abc"},
			wantErr: true,
		},
		{
			name:    "cost out of range",
			env:     map[string]string{"JWT_SECRET": "s", "DB_URL": "postgres://x", "BCRYPT_COST": "99"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := fromEnv(envFrom(tc.env))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Port != tc.wantPort {
				t.Errorf("port = %q, want %q", cfg.Port, tc.wantPort)
			}
			if cfg.BcryptCost != tc.wantCost {
				t.Errorf("cost = %d, want %d", cfg.BcryptCost, tc.wantCost)
			}
		})
	}
}

func TestFromEnvInviteCode(t *testing.T) {
	base := map[string]string{"JWT_SECRET": "s", "DB_URL": "postgres://x"}

	cfg, err := fromEnv(envFrom(base))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InviteCode != "" {
		t.Errorf("InviteCode = %q, want empty (registration closed by default)", cfg.InviteCode)
	}

	base["REGISTER_INVITE_CODE"] = "  team-code \n"
	cfg, err = fromEnv(envFrom(base))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InviteCode != "team-code" {
		t.Errorf("InviteCode = %q, want %q", cfg.InviteCode, "team-code")
	}
}

func baseEnv(extra map[string]string) map[string]string {
	env := map[string]string{"JWT_SECRET": "s", "DB_URL": "postgres://x"}
	for k, v := range extra {
		env[k] = v
	}
	return env
}

func TestAuthDefaults(t *testing.T) {
	cfg, err := fromEnv(envFrom(baseEnv(nil)))
	if err != nil {
		t.Fatalf("fromEnv: %v", err)
	}
	if cfg.TokenTTL != 15*time.Minute || cfg.RefreshTTL != 7*24*time.Hour {
		t.Errorf("ttls = %v / %v, want 15m / 168h", cfg.TokenTTL, cfg.RefreshTTL)
	}
	if !cfg.CookieSecure || cfg.CookieSameSite != http.SameSiteLaxMode {
		t.Errorf("cookie = secure:%v samesite:%v, want true/Lax (secure by default)", cfg.CookieSecure, cfg.CookieSameSite)
	}
	if len(cfg.CORSAllowedOrigins) != 1 || cfg.CORSAllowedOrigins[0] != "http://localhost:4200" {
		t.Errorf("cors origins = %v, want [http://localhost:4200]", cfg.CORSAllowedOrigins)
	}
}

func TestAuthOverrides(t *testing.T) {
	cfg, err := fromEnv(envFrom(baseEnv(map[string]string{
		"TOKEN_TTL":            "5m",
		"REFRESH_TOKEN_TTL":    "24h",
		"COOKIE_SECURE":        "true",
		"COOKIE_SAMESITE":      "None",
		"CORS_ALLOWED_ORIGINS": "https://a.example.com, http://localhost:4200",
	})))
	if err != nil {
		t.Fatalf("fromEnv: %v", err)
	}
	if cfg.TokenTTL != 5*time.Minute || cfg.RefreshTTL != 24*time.Hour {
		t.Errorf("ttls = %v / %v", cfg.TokenTTL, cfg.RefreshTTL)
	}
	if cfg.CookieSameSite != http.SameSiteNoneMode {
		t.Errorf("samesite = %v, want None", cfg.CookieSameSite)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[0] != "https://a.example.com" {
		t.Errorf("cors origins = %v", cfg.CORSAllowedOrigins)
	}
}

func TestAuthRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"ttl not a duration":    {"TOKEN_TTL": "soon"},
		"ttl zero":              {"REFRESH_TOKEN_TTL": "0s"},
		"ttl negative":          {"TOKEN_TTL": "-5m"},
		"secure not a bool":     {"COOKIE_SECURE": "maybe"},
		"unknown samesite":      {"COOKIE_SAMESITE": "sometimes"},
		"none without secure":   {"COOKIE_SAMESITE": "none", "COOKIE_SECURE": "false"},
		"wildcard origin":       {"CORS_ALLOWED_ORIGINS": "*"},
		"origin with path":      {"CORS_ALLOWED_ORIGINS": "https://a.example.com/app"},
		"origin without scheme": {"CORS_ALLOWED_ORIGINS": "a.example.com"},
		"only separators":       {"CORS_ALLOWED_ORIGINS": " , "},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := fromEnv(envFrom(baseEnv(extra))); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestLooseCookiesAreAllowedOverHTTPForLocalDev(t *testing.T) {
	cfg, err := fromEnv(envFrom(baseEnv(map[string]string{"COOKIE_SECURE": "false"})))
	if err != nil {
		t.Fatalf("fromEnv: %v", err)
	}
	if cfg.CookieSecure {
		t.Error("COOKIE_SECURE=false was ignored")
	}
}
