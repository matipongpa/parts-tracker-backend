package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"partlist/internal/identity/internal/repo"
	"partlist/internal/identity/internal/token"
)

const testInviteCode string = "team-invite"

type fakeStore struct {
	users map[string]repo.User
}

func newFakeStore() *fakeStore {
	return &fakeStore{users: map[string]repo.User{}}
}

func (f *fakeStore) Create(_ context.Context, u repo.User) error {
	if _, exists := f.users[u.Username]; exists {
		return repo.ErrDuplicate
	}
	f.users[u.Username] = u
	return nil
}

func (f *fakeStore) FindByUsername(_ context.Context, username string) (repo.User, error) {
	u, ok := f.users[username]
	if !ok {
		return repo.User{}, repo.ErrNotFound
	}
	return u, nil
}

func newTestService(t *testing.T) (*service, *fakeStore) {
	t.Helper()
	manager, err := token.NewManager("test-secret", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	store := newFakeStore()
	return &service{users: store, tokens: manager, refreshTTL: 24 * time.Hour, bcryptCost: bcrypt.MinCost, inviteCode: testInviteCode}, store
}

func TestNewRejectsEmptySecret(t *testing.T) {
	if _, err := New(nil, Config{JWTSecret: "", TokenTTL: time.Hour, RefreshTTL: time.Hour, BcryptCost: bcrypt.MinCost}); err == nil {
		t.Fatal("expected error for empty JWT secret")
	}
}

func TestRegisterHashesPassword(t *testing.T) {
	svc, store := newTestService(t)

	in := RegisterInput{InviteCode: testInviteCode, Username: "alice", Password: "pw", FirstName: "Alice", Surname: "A"}
	if err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("Register: %v", err)
	}

	stored := store.users["alice"]
	if stored.PasswordHash == "pw" {
		t.Fatal("password stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("pw")); err != nil {
		t.Errorf("stored hash does not match password: %v", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	svc, _ := newTestService(t)
	cases := map[string]RegisterInput{
		"empty username": {InviteCode: testInviteCode, Password: "pw"},
		"empty password": {InviteCode: testInviteCode, Username: "alice"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.Register(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRegisterRequiresInviteCode(t *testing.T) {
	svc, store := newTestService(t)
	cases := map[string]error{
		"":                   ErrInvalidInviteCode,
		"wrong":              ErrInvalidInviteCode,
		testInviteCode + "x": ErrInvalidInviteCode,
	}
	for code, want := range cases {
		in := RegisterInput{InviteCode: code, Username: "alice", Password: "pw"}
		if err := svc.Register(context.Background(), in); !errors.Is(err, want) {
			t.Errorf("code %q: err = %v, want %v", code, err, want)
		}
	}
	if len(store.users) != 0 {
		t.Fatalf("stored %d users after rejected registrations, want 0", len(store.users))
	}
}

func TestRegisterClosedWithoutConfiguredCode(t *testing.T) {
	svc, store := newTestService(t)
	svc.inviteCode = ""

	// An empty submitted code must not match an empty configured code.
	in := RegisterInput{InviteCode: "", Username: "alice", Password: "pw"}
	if err := svc.Register(context.Background(), in); !errors.Is(err, ErrRegistrationClosed) {
		t.Fatalf("err = %v, want ErrRegistrationClosed", err)
	}
	if len(store.users) != 0 {
		t.Fatal("user was stored while registration is closed")
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	svc, _ := newTestService(t)
	in := RegisterInput{InviteCode: testInviteCode, Username: "alice", Password: "pw"}

	if err := svc.Register(context.Background(), in); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := svc.Register(context.Background(), in); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("err = %v, want ErrUsernameTaken", err)
	}
}

func TestLoginThenVerify(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if err := svc.Register(ctx, RegisterInput{InviteCode: testInviteCode, Username: "alice", Password: "pw"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	session, err := svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if session.RefreshMaxAge != 24*time.Hour {
		t.Errorf("RefreshMaxAge = %v, want 24h", session.RefreshMaxAge)
	}
	claims, err := svc.Verify(ctx, session.AccessToken)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Username != "alice" {
		t.Errorf("username = %q, want alice", claims.Username)
	}
}

func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	if err := svc.Register(ctx, RegisterInput{InviteCode: testInviteCode, Username: "alice", Password: "pw"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := svc.Login(ctx, "alice", "wrong"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong password: err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Login(ctx, "nobody", "pw"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown user: err = %v, want ErrInvalidCredentials", err)
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.Verify(context.Background(), "nope"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func registerAlice(t *testing.T, svc *service) Session {
	t.Helper()
	ctx := context.Background()
	if err := svc.Register(ctx, RegisterInput{InviteCode: testInviteCode, Username: "alice", Password: "pw"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	session, err := svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	return session
}

func TestRefreshIssuesWorkingAccessToken(t *testing.T) {
	svc, _ := newTestService(t)
	session := registerAlice(t, svc)

	access, err := svc.Refresh(context.Background(), session.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	claims, err := svc.Verify(context.Background(), access)
	if err != nil || claims.Username != "alice" {
		t.Fatalf("Verify(refreshed) = %+v, %v; want alice", claims, err)
	}
}

func TestRefreshRejectsAccessTokenGarbageAndDeletedUser(t *testing.T) {
	svc, store := newTestService(t)
	session := registerAlice(t, svc)
	ctx := context.Background()

	if _, err := svc.Refresh(ctx, session.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("access token as refresh: err = %v, want ErrInvalidToken", err)
	}
	if _, err := svc.Refresh(ctx, "nope"); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("garbage: err = %v, want ErrInvalidToken", err)
	}

	delete(store.users, "alice")
	if _, err := svc.Refresh(ctx, session.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("deleted user: err = %v, want ErrInvalidToken", err)
	}
}

func TestVerifyRejectsRefreshToken(t *testing.T) {
	svc, _ := newTestService(t)
	session := registerAlice(t, svc)

	if _, err := svc.Verify(context.Background(), session.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken: a refresh token must not work as an access token", err)
	}
}
