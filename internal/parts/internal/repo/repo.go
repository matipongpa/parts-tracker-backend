package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrNotFound  error = errors.New("repo: part not found")
	ErrDuplicate error = errors.New("repo: part already exists")
)

const bulkBatchSize int = 500

// Row maps the existing "parts" table; column names are unchanged from before the refactor.
type Row struct {
	ID           uint64 `gorm:"column:id;primaryKey;autoIncrement"`
	PartNumber   string `gorm:"column:partnumber"`
	PcbRef       string `gorm:"column:pcbref"`
	Description  string `gorm:"column:description"`
	Footprint    string `gorm:"column:footprint"`
	Fit          string `gorm:"column:fit"`
	Manufacturer string `gorm:"column:manufacturer"`
	Type         string `gorm:"column:type"`
	ProjectName  string `gorm:"column:projectname"`
	Value        string `gorm:"column:value"`
}

func (Row) TableName() string { return "parts" }

// Filter holds substring filters (empty = no filter) and an already-normalized page.
type Filter struct {
	PcbRef      string
	PartNumber  string
	Value       string
	Description string
	Type        string
	Offset      int
	Limit       int
}

// Patch sets only the non-nil fields.
type Patch struct {
	PartNumber   *string
	PcbRef       *string
	Description  *string
	Footprint    *string
	Fit          *string
	Manufacturer *string
	Type         *string
	ProjectName  *string
	Value        *string
}

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Get(ctx context.Context, id uint64) (Row, error) {
	var row Row
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Row{}, ErrNotFound
		}
		return Row{}, fmt.Errorf("get part: %w", err)
	}
	return row, nil
}

// Search returns one page ordered by id plus the total number of matches.
func (r *Repository) Search(ctx context.Context, f Filter) ([]Row, int64, error) {
	q := r.db.WithContext(ctx).Model(&Row{})
	q = applyContains(q, "pcbref", f.PcbRef)
	q = applyContains(q, "partnumber", f.PartNumber)
	q = applyContains(q, "value", f.Value)
	q = applyContains(q, "description", f.Description)
	q = applyContains(q, "type", f.Type)

	var count int64
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, fmt.Errorf("count parts: %w", err)
	}

	rows := []Row{}
	if err := q.Order("id ASC").Offset(f.Offset).Limit(f.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("search parts: %w", err)
	}
	return rows, count, nil
}

// ByPartNumber returns up to limit rows whose part number matches case-insensitively (a substring
// unless exact), ordered by project, board and id so callers can group them without sorting.
func (r *Repository) ByPartNumber(ctx context.Context, partNumber string, exact bool, limit int) ([]Row, error) {
	pattern := escapeLike(partNumber)
	if !exact {
		pattern = "%" + pattern + "%"
	}
	rows := []Row{}
	err := r.db.WithContext(ctx).
		Where("partnumber ILIKE ?", pattern).
		Order("projectname ASC, pcbref ASC, id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("find parts by part number: %w", err)
	}
	return rows, nil
}

func (r *Repository) Create(ctx context.Context, row Row) (Row, error) {
	row.ID = 0
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return Row{}, ErrDuplicate
		}
		return Row{}, fmt.Errorf("create part: %w", err)
	}
	return row, nil
}

// Update applies the patch and returns the stored row.
func (r *Repository) Update(ctx context.Context, id uint64, p Patch) (Row, error) {
	values, columns := p.assignments()
	if len(columns) == 0 {
		return r.Get(ctx, id)
	}

	res := r.db.WithContext(ctx).Model(&Row{}).Where("id = ?", id).Select(columns).Updates(values)
	if errors.Is(res.Error, gorm.ErrDuplicatedKey) {
		return Row{}, ErrDuplicate
	}
	if res.Error != nil {
		return Row{}, fmt.Errorf("update part: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return Row{}, ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *Repository) Delete(ctx context.Context, id uint64) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&Row{})
	if res.Error != nil {
		return fmt.Errorf("delete part: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// BulkCreate inserts all rows in one transaction: either every row is stored or none.
// Rows that duplicate an existing part (or an earlier row in the batch) are skipped, and
// the returned count is the number actually inserted.
func (r *Repository) BulkCreate(ctx context.Context, rows []Row) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	batch := make([]Row, len(rows))
	for i, row := range rows {
		row.ID = 0
		batch[i] = row
	}
	var inserted int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&batch, bulkBatchSize)
		inserted = res.RowsAffected
		return res.Error
	})
	if err != nil {
		return 0, fmt.Errorf("bulk create parts: %w", err)
	}
	return int(inserted), nil
}

func (p Patch) assignments() (Row, []string) {
	var row Row
	columns := []string{}
	set := func(dst *string, src *string, column string) {
		if src != nil {
			*dst = *src
			columns = append(columns, column)
		}
	}
	set(&row.PartNumber, p.PartNumber, "partnumber")
	set(&row.PcbRef, p.PcbRef, "pcbref")
	set(&row.Description, p.Description, "description")
	set(&row.Footprint, p.Footprint, "footprint")
	set(&row.Fit, p.Fit, "fit")
	set(&row.Manufacturer, p.Manufacturer, "manufacturer")
	set(&row.Type, p.Type, "type")
	set(&row.ProjectName, p.ProjectName, "projectname")
	set(&row.Value, p.Value, "value")
	return row, columns
}

// applyContains adds a case-insensitive substring match; the pg_trgm GIN indexes serve ILIKE.
func applyContains(q *gorm.DB, column string, term string) *gorm.DB {
	if term == "" {
		return q
	}
	return q.Where(column+" ILIKE ?", "%"+escapeLike(term)+"%")
}

var likeEscaper *strings.Replacer = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}
