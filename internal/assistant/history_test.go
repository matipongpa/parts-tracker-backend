package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"partlist/internal/assistant/internal/history"
	"partlist/internal/parts"
)

type fakeStore struct {
	created history.Row
	keep    int
	err     error
}

func (f *fakeStore) Create(_ context.Context, row history.Row, keep int) (history.Row, error) {
	if f.err != nil {
		return history.Row{}, f.err
	}
	f.keep = keep
	row.ID = 42
	row.CreatedAt = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	f.created = row
	return row, nil
}

func (f *fakeStore) List(context.Context, string, int) ([]history.Summary, error) { return nil, nil }

func (f *fakeStore) Get(_ context.Context, _ string, id uint64) (history.Row, error) {
	if id != 42 {
		return history.Row{}, history.ErrNotFound
	}
	return history.Row{ID: 42, Question: "q", Answer: "a"}, nil
}

func (f *fakeStore) Delete(_ context.Context, _ string, id uint64) error {
	if id != 42 {
		return history.ErrNotFound
	}
	return nil
}

func TestSavePassesEverythingAndTheLimit(t *testing.T) {
	store := &fakeStore{}
	svc := &historyService{store: store, keep: 100}

	entry, err := svc.Save(context.Background(), "alice", "q?", Answer{
		Answer: "a", Parts: []parts.Part{{ID: "1"}}, PartsTruncated: true, ToolsUsed: []string{"search_parts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.ID != "42" || entry.Question != "q?" || !entry.PartsTruncated || len(entry.Parts) != 1 {
		t.Errorf("entry = %+v", entry)
	}
	if store.created.Username != "alice" || store.keep != 100 {
		t.Errorf("store got username %q keep %d", store.created.Username, store.keep)
	}
}

func TestSaveWrapsStoreErrors(t *testing.T) {
	boom := errors.New("boom")
	_, err := (&historyService{store: &fakeStore{err: boom}, keep: 100}).Save(context.Background(), "alice", "q", Answer{})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the store error", err)
	}
}

func TestGetAndDeleteTreatBadAndMissingIDsAsNotFound(t *testing.T) {
	svc := &historyService{store: &fakeStore{}, keep: 100}
	ctx := context.Background()
	for _, id := range []string{"abc", "-1", "", "1.5", "99999999999999999999999", "7"} {
		if _, err := svc.Get(ctx, "alice", id); !errors.Is(err, ErrHistoryNotFound) {
			t.Errorf("Get(%q) err = %v, want ErrHistoryNotFound", id, err)
		}
		if err := svc.Delete(ctx, "alice", id); !errors.Is(err, ErrHistoryNotFound) {
			t.Errorf("Delete(%q) err = %v, want ErrHistoryNotFound", id, err)
		}
	}
	if _, err := svc.Get(ctx, "alice", "42"); err != nil {
		t.Errorf("Get(42): %v", err)
	}
	if err := svc.Delete(ctx, "alice", "42"); err != nil {
		t.Errorf("Delete(42): %v", err)
	}
}
