// Package store keeps the column mappings the team has confirmed, one per header fingerprint.
package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"partlist/internal/bomconverter/internal/layout"
)

// Row maps bom_mappings. The mapping is shared by everyone; UpdatedBy only records who confirmed it last.
type Row struct {
	Fingerprint string         `gorm:"column:fingerprint;primaryKey"`
	Mapping     layout.Mapping `gorm:"column:mapping;type:jsonb;serializer:json"`
	CreatedBy   string         `gorm:"column:created_by"`
	UpdatedBy   string         `gorm:"column:updated_by"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (Row) TableName() string { return "bom_mappings" }

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Find returns the saved mapping for a fingerprint; ok is false when there is none.
func (r *Repository) Find(ctx context.Context, fingerprint string) (layout.Mapping, bool, error) {
	// Find with a limit, not Take: a first-time layout is the normal case and Take would log "record not found" for it.
	var rows []Row
	if err := r.db.WithContext(ctx).Where("fingerprint = ?", fingerprint).Limit(1).Find(&rows).Error; err != nil {
		return layout.Mapping{}, false, fmt.Errorf("find mapping: %w", err)
	}
	if len(rows) == 0 {
		return layout.Mapping{}, false, nil
	}
	return rows[0].Mapping, true, nil
}

// Save stores the mapping, replacing any earlier one for the same fingerprint (last confirmation wins).
func (r *Repository) Save(ctx context.Context, fingerprint string, m layout.Mapping, user string) error {
	row := Row{Fingerprint: fingerprint, Mapping: m, CreatedBy: user, UpdatedBy: user}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "fingerprint"}},
		DoUpdates: clause.AssignmentColumns([]string{"mapping", "updated_by", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("save mapping: %w", err)
	}
	return nil
}
