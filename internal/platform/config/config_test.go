package config

import (
	"testing"

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
