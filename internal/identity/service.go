package identity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"partlist/internal/identity/internal/repo"
	"partlist/internal/identity/internal/token"
)

var (
	ErrInvalidInput       error = errors.New("identity: invalid input")
	ErrUsernameTaken      error = errors.New("identity: username already taken")
	ErrInvalidCredentials error = errors.New("identity: invalid username or password")
	ErrInvalidToken       error = errors.New("identity: invalid token")
	// ErrRegistrationClosed means no invite code is configured, so nobody can register.
	ErrRegistrationClosed error = errors.New("identity: registration is closed")
	ErrInvalidInviteCode  error = errors.New("identity: invalid invite code")
)

type Claims struct {
	Username string
}

// Session is what a successful login hands back.
type Session struct {
	// AccessToken is short-lived; the client sends it as a Bearer token and keeps it in memory.
	AccessToken string
	// RefreshToken only buys a new AccessToken. It belongs in an httpOnly cookie, never in JS.
	RefreshToken string
	// RefreshMaxAge is how long RefreshToken stays valid, for the cookie's Max-Age.
	RefreshMaxAge time.Duration
}

type RegisterInput struct {
	Username  string
	Password  string
	FirstName string
	Surname   string
	// InviteCode must match Config.InviteCode.
	InviteCode string
}

// Service is the public surface of the identity module.
type Service interface {
	Register(ctx context.Context, in RegisterInput) error
	// Login checks the credentials and starts a session.
	Login(ctx context.Context, username string, password string) (Session, error)
	// Refresh exchanges a refresh token for a new access token. It returns ErrInvalidToken
	// when the token is bad, expired, of the wrong kind, or its user no longer exists.
	Refresh(ctx context.Context, refreshToken string) (string, error)
	// Verify validates an access token.
	Verify(ctx context.Context, rawToken string) (Claims, error)
}

type Config struct {
	JWTSecret string
	// TokenTTL is the access token lifetime; keep it short because it cannot be revoked.
	TokenTTL time.Duration
	// RefreshTTL is the refresh token lifetime, i.e. the longest a login lasts without re-entering a password.
	RefreshTTL time.Duration
	BcryptCost int
	// InviteCode gates Register. Empty closes registration entirely.
	InviteCode string
}

type userStore interface {
	Create(ctx context.Context, u repo.User) error
	FindByUsername(ctx context.Context, username string) (repo.User, error)
}

type tokenManager interface {
	Issue(username string) (string, error)
	IssueRefresh(username string) (string, error)
	Parse(raw string) (string, error)
	ParseRefresh(raw string) (string, error)
}

type service struct {
	users      userStore
	tokens     tokenManager
	refreshTTL time.Duration
	bcryptCost int
	inviteCode string
}

// New wires the module. It fails fast on an empty JWT secret.
func New(db *gorm.DB, cfg Config) (Service, error) {
	manager, err := token.NewManager(cfg.JWTSecret, cfg.TokenTTL, cfg.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("identity: %w", err)
	}
	return &service{
		users:      repo.New(db),
		tokens:     manager,
		refreshTTL: cfg.RefreshTTL,
		bcryptCost: cfg.BcryptCost,
		inviteCode: cfg.InviteCode,
	}, nil
}

// checkInvite compares digests so the comparison time does not depend on the code's length.
func (s *service) checkInvite(code string) error {
	if s.inviteCode == "" {
		return ErrRegistrationClosed
	}
	want := sha256.Sum256([]byte(s.inviteCode))
	got := sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
		return ErrInvalidInviteCode
	}
	return nil
}

func (s *service) Register(ctx context.Context, in RegisterInput) error {
	// Checked first so an uninvited caller learns nothing about which usernames exist.
	if err := s.checkInvite(in.InviteCode); err != nil {
		return err
	}
	if in.Username == "" || in.Password == "" {
		return ErrInvalidInput
	}

	// Best-effort pre-check; the unique index (migrations/0001) is what closes the race.
	_, err := s.users.FindByUsername(ctx, in.Username)
	switch {
	case err == nil:
		return ErrUsernameTaken
	case !errors.Is(err, repo.ErrNotFound):
		return fmt.Errorf("check username: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), s.bcryptCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return ErrInvalidInput
		}
		return fmt.Errorf("hash password: %w", err)
	}

	err = s.users.Create(ctx, repo.User{
		Username:     in.Username,
		PasswordHash: string(hash),
		FirstName:    in.FirstName,
		Surname:      in.Surname,
	})
	if errors.Is(err, repo.ErrDuplicate) {
		return ErrUsernameTaken
	}
	return err
}

func (s *service) Login(ctx context.Context, username string, password string) (Session, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if errors.Is(err, repo.ErrNotFound) {
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, fmt.Errorf("find user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("compare password: %w", err)
	}

	access, err := s.tokens.Issue(user.Username)
	if err != nil {
		return Session{}, err
	}
	refresh, err := s.tokens.IssueRefresh(user.Username)
	if err != nil {
		return Session{}, err
	}
	return Session{AccessToken: access, RefreshToken: refresh, RefreshMaxAge: s.refreshTTL}, nil
}

func (s *service) Refresh(ctx context.Context, refreshToken string) (string, error) {
	username, err := s.tokens.ParseRefresh(refreshToken)
	if err != nil {
		return "", ErrInvalidToken
	}
	// A refresh outlives the password check, so confirm the account still exists.
	if _, err := s.users.FindByUsername(ctx, username); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", ErrInvalidToken
		}
		return "", fmt.Errorf("find user: %w", err)
	}
	return s.tokens.Issue(username)
}

func (s *service) Verify(_ context.Context, rawToken string) (Claims, error) {
	username, err := s.tokens.Parse(rawToken)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	return Claims{Username: username}, nil
}
