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
	manager, err := token.NewManager("test-secret", time.Hour)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	store := newFakeStore()
	return &service{users: store, tokens: manager, bcryptCost: bcrypt.MinCost}, store
}

func TestNewRejectsEmptySecret(t *testing.T) {
	if _, err := New(nil, Config{JWTSecret: "", TokenTTL: time.Hour, BcryptCost: bcrypt.MinCost}); err == nil {
		t.Fatal("expected error for empty JWT secret")
	}
}

func TestRegisterHashesPassword(t *testing.T) {
	svc, store := newTestService(t)

	in := RegisterInput{Username: "alice", Password: "pw", FirstName: "Alice", Surname: "A"}
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
		"empty username": {Password: "pw"},
		"empty password": {Username: "alice"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.Register(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRegisterDuplicateUsername(t *testing.T) {
	svc, _ := newTestService(t)
	in := RegisterInput{Username: "alice", Password: "pw"}

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
	if err := svc.Register(ctx, RegisterInput{Username: "alice", Password: "pw"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	raw, err := svc.Login(ctx, "alice", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	claims, err := svc.Verify(ctx, raw)
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
	if err := svc.Register(ctx, RegisterInput{Username: "alice", Password: "pw"}); err != nil {
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
