package assistant

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"partlist/internal/assistant/internal/history"
	"partlist/internal/parts"
)

// DefaultHistoryLimit is how many answered questions are kept per user; older ones are deleted on save.
const DefaultHistoryLimit int = 100

// ErrHistoryNotFound covers a missing entry, one that belongs to another user, and a malformed id.
var ErrHistoryNotFound error = errors.New("assistant: history entry not found")

// Entry is one saved conversation turn: the question exactly as asked and the answer exactly as given,
// with the parts as they were at that moment. ID is empty when saving failed (the answer is still returned).
type Entry struct {
	ID             string       `json:"id"`
	Question       string       `json:"question"`
	Answer         string       `json:"answer"`
	Parts          []parts.Part `json:"parts"`
	PartsTruncated bool         `json:"parts_truncated"`
	ToolsUsed      []string     `json:"tools_used"`
	CreatedAt      time.Time    `json:"created_at"`
}

// HistoryItem is a list entry; open it with Get for the full answer and parts.
type HistoryItem struct {
	ID        string    `json:"id"`
	Question  string    `json:"question"`
	CreatedAt time.Time `json:"created_at"`
}

// History stores answered questions per user. Every method is scoped to the username it is given.
type History interface {
	Save(ctx context.Context, username string, question string, answer Answer) (Entry, error)
	List(ctx context.Context, username string) ([]HistoryItem, error)
	Get(ctx context.Context, username string, id string) (Entry, error)
	Delete(ctx context.Context, username string, id string) error
}

type historyStore interface {
	Create(ctx context.Context, row history.Row, keep int) (history.Row, error)
	List(ctx context.Context, username string, limit int) ([]history.Summary, error)
	Get(ctx context.Context, username string, id uint64) (history.Row, error)
	Delete(ctx context.Context, username string, id uint64) error
}

type historyService struct {
	store historyStore
	keep  int
}

// NewHistory returns a History that keeps the newest keep entries per user.
func NewHistory(db *gorm.DB, keep int) History {
	return &historyService{store: history.New(db), keep: keep}
}

func entryFromRow(r history.Row) Entry {
	return Entry{
		ID:             strconv.FormatUint(r.ID, 10),
		Question:       r.Question,
		Answer:         r.Answer,
		Parts:          r.Parts,
		PartsTruncated: r.PartsTruncated,
		ToolsUsed:      r.ToolsUsed,
		CreatedAt:      r.CreatedAt,
	}
}

func parseID(id string) (uint64, error) {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return 0, ErrHistoryNotFound
	}
	return n, nil
}

func (s *historyService) Save(ctx context.Context, username string, question string, a Answer) (Entry, error) {
	row, err := s.store.Create(ctx, history.Row{
		Username:       username,
		Question:       question,
		Answer:         a.Answer,
		ToolsUsed:      a.ToolsUsed,
		Parts:          a.Parts,
		PartsTruncated: a.PartsTruncated,
	}, s.keep)
	if err != nil {
		return Entry{}, fmt.Errorf("save history: %w", err)
	}
	return entryFromRow(row), nil
}

func (s *historyService) List(ctx context.Context, username string) ([]HistoryItem, error) {
	rows, err := s.store.List(ctx, username, s.keep)
	if err != nil {
		return nil, err
	}
	items := make([]HistoryItem, len(rows))
	for i, r := range rows {
		items[i] = HistoryItem{ID: strconv.FormatUint(r.ID, 10), Question: r.Question, CreatedAt: r.CreatedAt}
	}
	return items, nil
}

func (s *historyService) Get(ctx context.Context, username string, id string) (Entry, error) {
	key, err := parseID(id)
	if err != nil {
		return Entry{}, err
	}
	row, err := s.store.Get(ctx, username, key)
	if errors.Is(err, history.ErrNotFound) {
		return Entry{}, ErrHistoryNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	return entryFromRow(row), nil
}

func (s *historyService) Delete(ctx context.Context, username string, id string) error {
	key, err := parseID(id)
	if err != nil {
		return err
	}
	err = s.store.Delete(ctx, username, key)
	if errors.Is(err, history.ErrNotFound) {
		return ErrHistoryNotFound
	}
	return err
}
