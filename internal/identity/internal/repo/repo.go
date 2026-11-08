package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	ErrNotFound  error = errors.New("repo: user not found")
	ErrDuplicate error = errors.New("repo: username already exists")
)

// User maps the existing "users" table; column names are unchanged from before the refactor.
type User struct {
	Username     string `gorm:"column:username"`
	PasswordHash string `gorm:"column:password"`
	FirstName    string `gorm:"column:firstname"`
	Surname      string `gorm:"column:surname"`
}

func (User) TableName() string { return "users" }

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, u User) error {
	if err := r.db.WithContext(ctx).Create(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ErrDuplicate
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (r *Repository) FindByUsername(ctx context.Context, username string) (User, error) {
	var u User
	// Take, not First: the table has no primary key to order by.
	if err := r.db.WithContext(ctx).Where("username = ?", username).Take(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("find user: %w", err)
	}
	return u, nil
}
