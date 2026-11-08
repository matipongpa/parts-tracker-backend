package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrEmptySecret error = errors.New("token: secret must not be empty")
	ErrInvalidTTL  error = errors.New("token: ttl must be positive")
	ErrInvalid     error = errors.New("token: invalid")
)

// kind separates the two token types so a long-lived refresh token can never be
// used as an access token, and the other way round.
type kind string

const (
	// kindAccess is the empty string on purpose: tokens issued before refresh tokens existed carry no "typ".
	kindAccess  kind = ""
	kindRefresh kind = "refresh"
)

// claims keeps the "userName" JSON key so tokens issued before the refactor stay readable.
type claims struct {
	UserName string `json:"userName"`
	Kind     kind   `json:"typ,omitempty"`
	jwt.RegisteredClaims
}

// Manager signs and verifies HS256 tokens. The secret is injected, never read from package state.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

func NewManager(secret string, accessTTL time.Duration, refreshTTL time.Duration) (*Manager, error) {
	if secret == "" {
		return nil, ErrEmptySecret
	}
	if accessTTL <= 0 || refreshTTL <= 0 {
		return nil, ErrInvalidTTL
	}
	return &Manager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}, nil
}

// Issue signs a short-lived access token.
func (m *Manager) Issue(username string) (string, error) {
	return m.sign(username, kindAccess, m.accessTTL)
}

// IssueRefresh signs a long-lived token that can only be exchanged for a new access token.
func (m *Manager) IssueRefresh(username string) (string, error) {
	return m.sign(username, kindRefresh, m.refreshTTL)
}

// Parse returns the username from a valid, unexpired access token. Refresh tokens are rejected.
func (m *Manager) Parse(raw string) (string, error) {
	return m.parse(raw, kindAccess)
}

// ParseRefresh returns the username from a valid, unexpired refresh token. Access tokens are rejected.
func (m *Manager) ParseRefresh(raw string) (string, error) {
	return m.parse(raw, kindRefresh)
}

func (m *Manager) sign(username string, k kind, ttl time.Duration) (string, error) {
	issuedAt := m.now()
	c := claims{
		UserName: username,
		Kind:     k,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(issuedAt.Add(ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func (m *Manager) parse(raw string, want kind) (string, error) {
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
	if !ok || !parsed.Valid || c.UserName == "" || c.Kind != want {
		return "", ErrInvalid
	}
	return c.UserName, nil
}
