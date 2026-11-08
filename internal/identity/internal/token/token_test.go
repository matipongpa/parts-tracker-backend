package token

import (
	"errors"
	"testing"
	"time"
)

func mustManager(t *testing.T, secret string) *Manager {
	t.Helper()
	m, err := NewManager(secret, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func TestNewManagerRejectsEmptySecret(t *testing.T) {
	if _, err := NewManager("", time.Hour, 24*time.Hour); !errors.Is(err, ErrEmptySecret) {
		t.Fatalf("err = %v, want ErrEmptySecret", err)
	}
}

func TestIssueParseRoundTrip(t *testing.T) {
	m := mustManager(t, "secret")

	raw, err := m.Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	got, err := m.Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got != "alice" {
		t.Errorf("username = %q, want alice", got)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	raw, err := mustManager(t, "secret-a").Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := mustManager(t, "secret-b").Parse(raw); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestParseRejectsExpired(t *testing.T) {
	issuer := mustManager(t, "secret")
	issuer.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	raw, err := issuer.Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if _, err := mustManager(t, "secret").Parse(raw); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := mustManager(t, "secret").Parse("not-a-token"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestNewManagerRejectsNonPositiveTTL(t *testing.T) {
	cases := map[string][2]time.Duration{
		"zero access":   {0, time.Hour},
		"zero refresh":  {time.Hour, 0},
		"negative both": {-time.Hour, -time.Hour},
	}
	for name, ttls := range cases {
		if _, err := NewManager("secret", ttls[0], ttls[1]); !errors.Is(err, ErrInvalidTTL) {
			t.Errorf("%s: err = %v, want ErrInvalidTTL", name, err)
		}
	}
}

func TestRefreshTokenRoundTrip(t *testing.T) {
	m := mustManager(t, "secret")

	raw, err := m.IssueRefresh("alice")
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}
	got, err := m.ParseRefresh(raw)
	if err != nil {
		t.Fatalf("ParseRefresh: %v", err)
	}
	if got != "alice" {
		t.Errorf("username = %q, want alice", got)
	}
}

func TestTokenKindsAreNotInterchangeable(t *testing.T) {
	m := mustManager(t, "secret")
	access, err := m.Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	refresh, err := m.IssueRefresh("alice")
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}

	if _, err := m.Parse(refresh); !errors.Is(err, ErrInvalid) {
		t.Errorf("Parse(refresh token): err = %v, want ErrInvalid", err)
	}
	if _, err := m.ParseRefresh(access); !errors.Is(err, ErrInvalid) {
		t.Errorf("ParseRefresh(access token): err = %v, want ErrInvalid", err)
	}
}

func TestRefreshTokenOutlivesAccessToken(t *testing.T) {
	issuer := mustManager(t, "secret")
	issuer.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	access, err := issuer.Issue("alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	refresh, err := issuer.IssueRefresh("alice")
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}

	verifier := mustManager(t, "secret")
	if _, err := verifier.Parse(access); !errors.Is(err, ErrInvalid) {
		t.Errorf("expired access token: err = %v, want ErrInvalid", err)
	}
	if _, err := verifier.ParseRefresh(refresh); err != nil {
		t.Errorf("refresh token should still be valid: %v", err)
	}
}
