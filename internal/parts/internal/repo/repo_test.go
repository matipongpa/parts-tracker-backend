package repo_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"gorm.io/gorm"

	"partlist/internal/parts/internal/repo"
	"partlist/internal/platform/db"
	"partlist/internal/platform/migrate"
	"partlist/migrations"
)

// newDB connects to TEST_DB_URL (a disposable Postgres) and applies all migrations.
// The test is skipped when the variable is unset so `go test ./...` works without Docker.
func newDB(t *testing.T) *gorm.DB {
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
	if err := conn.Exec("TRUNCATE parts RESTART IDENTITY").Error; err != nil {
		t.Fatal(err)
	}
	return conn
}

func row(pn string, ref string, typ string) repo.Row {
	return repo.Row{PartNumber: pn, PcbRef: ref, Type: typ, Fit: "FIT", Manufacturer: "M", ProjectName: "P", Value: "1u"}
}

func TestCRUD(t *testing.T) {
	ctx := context.Background()
	r := repo.New(newDB(t))

	created, err := r.Create(ctx, row("PN1", "C1", "Capacitor"))
	if err != nil || created.ID == 0 {
		t.Fatalf("create: %+v, %v", created, err)
	}

	empty := ""
	newPN := "PN2"
	updated, err := r.Update(ctx, created.ID, repo.Patch{PartNumber: &newPN, Fit: &empty})
	if err != nil || updated.PartNumber != "PN2" || updated.Fit != "" || updated.Manufacturer != "M" {
		t.Fatalf("update: %+v, %v", updated, err)
	}

	if _, err := r.Update(ctx, 9999, repo.Patch{PartNumber: &newPN}); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if err := r.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	if _, err := r.Get(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("get deleted: %v", err)
	}
}

func TestSearchFiltersAndPaginates(t *testing.T) {
	ctx := context.Background()
	r := repo.New(newDB(t))
	if _, err := r.BulkCreate(ctx, []repo.Row{
		row("AAA", "C1", "Capacitor"), row("AAB", "C2", "Capacitor"), row("100%", "R1", "Resistor"),
	}); err != nil {
		t.Fatal(err)
	}

	rows, count, err := r.Search(ctx, repo.Filter{Type: "capac", Limit: 1})
	if err != nil || count != 2 || len(rows) != 1 || rows[0].PartNumber != "AAA" {
		t.Fatalf("page 0: %+v count=%d err=%v", rows, count, err)
	}
	rows, _, _ = r.Search(ctx, repo.Filter{Type: "capac", Offset: 1, Limit: 1})
	if len(rows) != 1 || rows[0].PartNumber != "AAB" {
		t.Errorf("page 1: %+v", rows)
	}

	// '%' is a literal, not a wildcard.
	_, count, _ = r.Search(ctx, repo.Filter{PartNumber: "%", Limit: 10})
	if count != 1 {
		t.Errorf("literal %% count = %d, want 1", count)
	}
}

func TestBulkCreateIsAtomic(t *testing.T) {
	ctx := context.Background()
	conn := newDB(t)
	r := repo.New(conn)

	// A CHECK constraint makes the second row fail so we can observe the rollback.
	if err := conn.Exec("ALTER TABLE parts ADD CONSTRAINT no_bad CHECK (partnumber <> 'BAD')").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Exec("ALTER TABLE parts DROP CONSTRAINT no_bad") })

	if _, err := r.BulkCreate(ctx, []repo.Row{row("OK", "C1", "Capacitor"), row("BAD", "C2", "Capacitor")}); err == nil {
		t.Fatal("expected error")
	}
	_, count, err := r.Search(ctx, repo.Filter{Limit: 10})
	if err != nil || count != 0 {
		t.Fatalf("count = %d, err = %v; want 0 (rolled back)", count, err)
	}
}

func TestDuplicatesAreRejectedOrSkipped(t *testing.T) {
	ctx := context.Background()
	r := repo.New(newDB(t))

	if _, err := r.Create(ctx, row("PN", "C1", "Capacitor")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, row("PN", "C1", "Capacitor")); !errors.Is(err, repo.ErrDuplicate) {
		t.Errorf("duplicate create: %v", err)
	}

	// One existing, one repeated within the batch, one new.
	n, err := r.BulkCreate(ctx, []repo.Row{row("PN", "C1", "Capacitor"), row("NEW", "C2", "Capacitor"), row("NEW", "C2", "Capacitor")})
	if err != nil || n != 1 {
		t.Fatalf("inserted = %d, err = %v; want 1", n, err)
	}
	_, count, _ := r.Search(ctx, repo.Filter{Limit: 10})
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	other, _ := r.Create(ctx, row("OTHER", "C9", "Capacitor"))
	pn := "PN"
	ref := "C1"
	if _, err := r.Update(ctx, other.ID, repo.Patch{PartNumber: &pn, PcbRef: &ref}); !errors.Is(err, repo.ErrDuplicate) {
		t.Errorf("update into duplicate: %v", err)
	}
}
