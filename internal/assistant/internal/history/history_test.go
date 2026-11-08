package history_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"gorm.io/gorm"

	"partlist/internal/assistant/internal/history"
	"partlist/internal/parts"
	"partlist/internal/platform/db"
	"partlist/internal/platform/migrate"
	"partlist/migrations"
)

// newRepo connects to TEST_DB_URL (a disposable Postgres) and applies all migrations.
// Skipped when the variable is unset so `go test ./...` works without Docker.
func newRepo(t *testing.T) (*history.Repository, *gorm.DB) {
	t.Helper()
	url := os.Getenv("TEST_DB_URL")
	if url == "" {
		t.Skip("TEST_DB_URL not set")
	}
	conn, err := db.Open(url)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(conn, migrations.FS); err != nil {
		t.Fatal(err)
	}
	if err := conn.Exec("TRUNCATE ask_history RESTART IDENTITY").Error; err != nil {
		t.Fatal(err)
	}
	return history.New(conn), conn
}

func row(username string, question string) history.Row {
	return history.Row{Username: username, Question: question, Answer: "answer to " + question}
}

func TestCreateGetRoundTripKeepsThePartsSnapshot(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	in := row("alice", "q")
	in.ToolsUsed = []string{"search_parts"}
	in.PartsTruncated = true
	in.Parts = []parts.Part{{ID: "1", PartNumber: "GRM188", ProjectName: "Alpha", PcbRef: "A1"}}

	created, err := repo.Create(ctx, in, 100)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, "alice", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Question != "q" || got.Answer != "answer to q" || !got.PartsTruncated || got.CreatedAt.IsZero() {
		t.Errorf("got = %+v", got)
	}
	if len(got.Parts) != 1 || got.Parts[0].PartNumber != "GRM188" || got.Parts[0].ProjectName != "Alpha" {
		t.Errorf("parts snapshot = %+v", got.Parts)
	}
	if len(got.ToolsUsed) != 1 || got.ToolsUsed[0] != "search_parts" {
		t.Errorf("tools = %+v", got.ToolsUsed)
	}
}

func TestNilSlicesAreStoredAsEmptyArrays(t *testing.T) {
	repo, conn := newRepo(t)
	if _, err := repo.Create(context.Background(), row("alice", "q"), 100); err != nil {
		t.Fatal(err)
	}
	var parts, tools string
	if err := conn.Raw("SELECT parts::text, tools_used::text FROM ask_history").Row().Scan(&parts, &tools); err != nil {
		t.Fatal(err)
	}
	if parts != "[]" || tools != "[]" {
		t.Errorf("stored parts %q tools %q, want []", parts, tools)
	}
}

func TestKeepsOnlyTheNewestEntriesPerUser(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	const keep int = 3
	for i := 1; i <= 5; i++ {
		if _, err := repo.Create(ctx, row("alice", fmt.Sprintf("alice %d", i)), keep); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.Create(ctx, row("bob", "bob 1"), keep); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(ctx, "alice", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != keep || list[0].Question != "alice 5" || list[keep-1].Question != "alice 3" {
		t.Errorf("alice's list = %+v, want the newest %d, newest first", list, keep)
	}
	if bob, _ := repo.List(ctx, "bob", 100); len(bob) != 1 {
		t.Errorf("bob lost entries to alice's pruning: %+v", bob)
	}
}

func TestEntriesAreScopedToTheirOwner(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, row("alice", "private"), 100)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.Get(ctx, "bob", created.ID); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("bob Get: err = %v, want ErrNotFound", err)
	}
	if err := repo.Delete(ctx, "bob", created.ID); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("bob Delete: err = %v, want ErrNotFound", err)
	}
	if _, err := repo.Get(ctx, "alice", created.ID); err != nil {
		t.Errorf("alice lost her entry: %v", err)
	}
	if list, _ := repo.List(ctx, "bob", 100); len(list) != 0 {
		t.Errorf("bob's list = %+v, want empty", list)
	}
}

func TestDelete(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()
	created, err := repo.Create(ctx, row("alice", "q"), 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "alice", created.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "alice", created.ID); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("second delete: err = %v, want ErrNotFound", err)
	}
}
