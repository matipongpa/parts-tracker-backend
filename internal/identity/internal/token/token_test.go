package token

import (
	"errors"
	"testing"
	"time"
)

func mustManager(t *testing.T, secret string) *Manager {
	t.Helper()
	m, err := NewManager(secret, time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func TestNewManagerRejectsEmptySecret(t *testing.T) {
	if _, err := NewManager("", time.Hour); !errors.Is(err, ErrEmptySecret) {
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
