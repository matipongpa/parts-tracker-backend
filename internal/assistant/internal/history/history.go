// Package history persists answered assistant questions per user.
package history

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"partlist/internal/parts"
)

var ErrNotFound error = errors.New("history: entry not found")

// Row maps ask_history. Parts and ToolsUsed are stored as JSONB snapshots.
type Row struct {
	ID             uint64       `gorm:"column:id;primaryKey"`
	Username       string       `gorm:"column:username"`
	Question       string       `gorm:"column:question"`
	Answer         string       `gorm:"column:answer"`
	ToolsUsed      []string     `gorm:"column:tools_used;type:jsonb;serializer:json"`
	Parts          []parts.Part `gorm:"column:parts;type:jsonb;serializer:json"`
	PartsTruncated bool         `gorm:"column:parts_truncated"`
	CreatedAt      time.Time    `gorm:"column:created_at;autoCreateTime"`
}

func (Row) TableName() string { return "ask_history" }

// Summary is a list item: enough to find an entry without loading its parts.
type Summary struct {
	ID        uint64    `gorm:"column:id"`
	Question  string    `gorm:"column:question"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts the row and, in the same transaction, deletes the user's entries beyond the newest keep.
func (r *Repository) Create(ctx context.Context, row Row, keep int) (Row, error) {
	// A nil slice would be stored as JSON null instead of [].
	if row.ToolsUsed == nil {
		row.ToolsUsed = []string{}
	}
	if row.Parts == nil {
		row.Parts = []parts.Part{}
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("insert entry: %w", err)
		}
		const prune string = `DELETE FROM ask_history WHERE username = ? AND id NOT IN (
			SELECT id FROM ask_history WHERE username = ? ORDER BY id DESC LIMIT ?)`
		if err := tx.Exec(prune, row.Username, row.Username, keep).Error; err != nil {
			return fmt.Errorf("prune old entries: %w", err)
		}
		return nil
	})
	if err != nil {
		return Row{}, err
	}
	return row, nil
}

// List returns the user's newest entries first.
func (r *Repository) List(ctx context.Context, username string, limit int) ([]Summary, error) {
	out := []Summary{}
	err := r.db.WithContext(ctx).Model(&Row{}).
		Select("id", "question", "created_at").
		Where("username = ?", username).
		Order("id DESC").Limit(limit).
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	return out, nil
}

// Get returns one entry, only if it belongs to username.
func (r *Repository) Get(ctx context.Context, username string, id uint64) (Row, error) {
	var row Row
	err := r.db.WithContext(ctx).Where("id = ? AND username = ?", id, username).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Row{}, ErrNotFound
	}
	if err != nil {
		return Row{}, fmt.Errorf("get entry: %w", err)
	}
	return row, nil
}

// Delete removes one entry, only if it belongs to username.
func (r *Repository) Delete(ctx context.Context, username string, id uint64) error {
	res := r.db.WithContext(ctx).Where("id = ? AND username = ?", id, username).Delete(&Row{})
	if res.Error != nil {
		return fmt.Errorf("delete entry: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
