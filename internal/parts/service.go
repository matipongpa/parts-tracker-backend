package parts

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"partlist/internal/parts/internal/repo"
)

var (
	ErrInvalidInput error = errors.New("parts: invalid input")
	ErrNotFound     error = errors.New("parts: part not found")
	ErrDuplicate    error = errors.New("parts: part already exists")
)

const (
	defaultPageSize int = 10
	maxPageSize     int = 100
	usageMaxRows    int = 500
)

type Part struct {
	ID           string `json:"id"`
	PartNumber   string `json:"part_number"`
	PcbRef       string `json:"pcb_ref"`
	Description  string `json:"description"`
	Footprint    string `json:"footprint"`
	Fit          string `json:"fit"`
	Manufacturer string `json:"manufacturer"`
	Type         string `json:"type"`
	ProjectName  string `json:"project_name"`
	Value        string `json:"value"`
}

// NewPart is the input for Create and BulkCreate. Description and Footprint are optional.
type NewPart struct {
	PartNumber   string `json:"part_number"`
	PcbRef       string `json:"pcb_ref"`
	Description  string `json:"description"`
	Footprint    string `json:"footprint"`
	Fit          string `json:"fit"`
	Manufacturer string `json:"manufacturer"`
	Type         string `json:"type"`
	ProjectName  string `json:"project_name"`
	Value        string `json:"value"`
}

// Validate reports ErrInvalidInput when a required field is empty.
func (p NewPart) Validate() error { return validateNew(p) }

// PartPatch updates only the fields that are present (non-nil); an empty string is a valid value.
type PartPatch struct {
	PartNumber   *string `json:"part_number"`
	PcbRef       *string `json:"pcb_ref"`
	Description  *string `json:"description"`
	Footprint    *string `json:"footprint"`
	Fit          *string `json:"fit"`
	Manufacturer *string `json:"manufacturer"`
	Type         *string `json:"type"`
	ProjectName  *string `json:"project_name"`
	Value        *string `json:"value"`
}

// SearchQuery filters are case-insensitive substring matches. Page is zero-based.
type SearchQuery struct {
	PcbRef      string
	PartNumber  string
	Value       string
	Description string
	Type        string
	Page        int
	PageSize    int
}

type SearchResult struct {
	List  []Part `json:"list"`
	Count int64  `json:"count"`
}

// ProjectUsage lists the stored parts, one per board, that match a part number within one project.
type ProjectUsage struct {
	ProjectName string `json:"project_name"`
	Parts       []Part `json:"parts"`
}

// UsageResult answers "where is this part number used?", grouped by project.
// Matches counts the returned parts; Truncated is set when more exist than usageMaxRows.
type UsageResult struct {
	PartNumber string         `json:"part_number"`
	Projects   []ProjectUsage `json:"projects"`
	Matches    int            `json:"matches"`
	Truncated  bool           `json:"truncated"`
}

// Service is the public surface of the parts module.
type Service interface {
	Get(ctx context.Context, id string) (Part, error)
	Search(ctx context.Context, q SearchQuery) (SearchResult, error)
	// Usage finds a part number across every project. Matching is case-insensitive; a substring
	// unless exact. A blank part number is ErrInvalidInput.
	Usage(ctx context.Context, partNumber string, exact bool) (UsageResult, error)
	Create(ctx context.Context, p NewPart) (Part, error)
	Update(ctx context.Context, id string, p PartPatch) (Part, error)
	Delete(ctx context.Context, id string) error
	// BulkCreate stores the parts in one transaction and returns how many were inserted.
	// Parts that already exist (same pcb_ref, part_number, project_name) are skipped.
	BulkCreate(ctx context.Context, ps []NewPart) (int, error)
	ClassifyDesignator(designator string) PartType
}

type partStore interface {
	Get(ctx context.Context, id uint64) (repo.Row, error)
	Search(ctx context.Context, f repo.Filter) ([]repo.Row, int64, error)
	ByPartNumber(ctx context.Context, partNumber string, exact bool, limit int) ([]repo.Row, error)
	Create(ctx context.Context, row repo.Row) (repo.Row, error)
	Update(ctx context.Context, id uint64, p repo.Patch) (repo.Row, error)
	Delete(ctx context.Context, id uint64) error
	BulkCreate(ctx context.Context, rows []repo.Row) (int, error)
}

type service struct {
	store partStore
}

// Options configures the in-process search cache. MaxEntries <= 0 disables it.
type Options struct {
	CacheMaxEntries int
	CacheTTL        time.Duration
}

func New(db *gorm.DB, opts Options) Service {
	var svc Service = &service{store: repo.New(db)}
	if opts.CacheMaxEntries > 0 && opts.CacheTTL > 0 {
		svc = newCachedService(svc, newLRUCache(opts.CacheMaxEntries, opts.CacheTTL))
	}
	return svc
}

func (s *service) Get(ctx context.Context, id string) (Part, error) {
	key, err := parseID(id)
	if err != nil {
		return Part{}, err
	}
	row, err := s.store.Get(ctx, key)
	if err != nil {
		return Part{}, mapRepoErr(err)
	}
	return toPart(row), nil
}

func (s *service) Search(ctx context.Context, q SearchQuery) (SearchResult, error) {
	offset, limit := normalizePage(q.Page, q.PageSize)
	rows, count, err := s.store.Search(ctx, repo.Filter{
		PcbRef:      q.PcbRef,
		PartNumber:  q.PartNumber,
		Value:       q.Value,
		Description: q.Description,
		Type:        q.Type,
		Offset:      offset,
		Limit:       limit,
	})
	if err != nil {
		return SearchResult{}, err
	}
	list := make([]Part, len(rows))
	for i, row := range rows {
		list[i] = toPart(row)
	}
	return SearchResult{List: list, Count: count}, nil
}

func (s *service) Usage(ctx context.Context, partNumber string, exact bool) (UsageResult, error) {
	term := strings.TrimSpace(partNumber)
	if term == "" {
		return UsageResult{}, ErrInvalidInput
	}
	// One extra row tells us whether the cap cut the result short.
	rows, err := s.store.ByPartNumber(ctx, term, exact, usageMaxRows+1)
	if err != nil {
		return UsageResult{}, err
	}
	truncated := len(rows) > usageMaxRows
	if truncated {
		rows = rows[:usageMaxRows]
	}
	return groupUsage(term, rows, truncated), nil
}

// groupUsage groups rows by project, keeping the first-seen order of projects and rows.
func groupUsage(partNumber string, rows []repo.Row, truncated bool) UsageResult {
	projects := []ProjectUsage{}
	index := map[string]int{}
	for _, row := range rows {
		i, seen := index[row.ProjectName]
		if !seen {
			i = len(projects)
			index[row.ProjectName] = i
			projects = append(projects, ProjectUsage{ProjectName: row.ProjectName, Parts: []Part{}})
		}
		projects[i].Parts = append(projects[i].Parts, toPart(row))
	}
	return UsageResult{PartNumber: partNumber, Projects: projects, Matches: len(rows), Truncated: truncated}
}

func (s *service) Create(ctx context.Context, p NewPart) (Part, error) {
	if err := validateNew(p); err != nil {
		return Part{}, err
	}
	row, err := s.store.Create(ctx, toRow(p))
	if err != nil {
		return Part{}, mapRepoErr(err)
	}
	return toPart(row), nil
}

func (s *service) Update(ctx context.Context, id string, p PartPatch) (Part, error) {
	key, err := parseID(id)
	if err != nil {
		return Part{}, err
	}
	row, err := s.store.Update(ctx, key, repo.Patch(p))
	if err != nil {
		return Part{}, mapRepoErr(err)
	}
	return toPart(row), nil
}

func (s *service) Delete(ctx context.Context, id string) error {
	key, err := parseID(id)
	if err != nil {
		return err
	}
	return mapRepoErr(s.store.Delete(ctx, key))
}

func (s *service) BulkCreate(ctx context.Context, ps []NewPart) (int, error) {
	rows := make([]repo.Row, len(ps))
	for i, p := range ps {
		if err := validateNew(p); err != nil {
			return 0, fmt.Errorf("part %d: %w", i, err)
		}
		rows[i] = toRow(p)
	}
	return s.store.BulkCreate(ctx, rows)
}

func (s *service) ClassifyDesignator(designator string) PartType {
	return classifyDesignator(designator)
}

// parseID treats anything that is not a positive integer as not found, since no such row can exist.
func parseID(id string) (uint64, error) {
	key, err := strconv.ParseUint(id, 10, 64)
	if err != nil || key == 0 {
		return 0, ErrNotFound
	}
	return key, nil
}

// normalizePage clamps to [defaultPageSize, maxPageSize] and returns a zero-based offset.
func normalizePage(page int, pageSize int) (int, int) {
	if page < 0 {
		page = 0
	}
	switch {
	case pageSize > maxPageSize:
		pageSize = maxPageSize
	case pageSize <= 0:
		pageSize = defaultPageSize
	}
	return page * pageSize, pageSize
}

func validateNew(p NewPart) error {
	if p.Value == "" || p.PartNumber == "" || p.PcbRef == "" || p.Fit == "" ||
		p.Manufacturer == "" || p.Type == "" || p.ProjectName == "" {
		return ErrInvalidInput
	}
	return nil
}

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return ErrNotFound
	case errors.Is(err, repo.ErrDuplicate):
		return ErrDuplicate
	}
	return err
}

func toRow(p NewPart) repo.Row {
	return repo.Row{
		PartNumber: p.PartNumber, PcbRef: p.PcbRef, Description: p.Description, Footprint: p.Footprint,
		Fit: p.Fit, Manufacturer: p.Manufacturer, Type: p.Type, ProjectName: p.ProjectName, Value: p.Value,
	}
}

func toPart(r repo.Row) Part {
	return Part{
		ID: strconv.FormatUint(r.ID, 10), PartNumber: r.PartNumber, PcbRef: r.PcbRef,
		Description: r.Description, Footprint: r.Footprint, Fit: r.Fit, Manufacturer: r.Manufacturer,
		Type: r.Type, ProjectName: r.ProjectName, Value: r.Value,
	}
}
