package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrEmptySecret error = errors.New("token: secret must not be empty")
	ErrInvalid     error = errors.New("token: invalid")
)

// claims keeps the "userName" JSON key so tokens issued before the refactor stay readable.
type claims struct {
	UserName string `json:"userName"`
	jwt.RegisteredClaims
}

// Manager signs and verifies HS256 tokens. The secret is injected, never read from package state.
type Manager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewManager(secret string, ttl time.Duration) (*Manager, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	return &Manager{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

func (m *Manager) Issue(username string) (string, error) {
	issuedAt := m.now()
	c := claims{
		UserName: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(m.ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse returns the username from a valid, unexpired token.
func (m *Manager) Parse(raw string) (string, error) {
	parsed, err := jwt.ParseWithClaims(
		raw,
		&claims{},
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}

	c, ok := parsed.Claims.(*claims)
	if !ok || !parsed.Valid || c.UserName == "" {
		return "", ErrInvalid
	}
	return c.UserName, nil
}
