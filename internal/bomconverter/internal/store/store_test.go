package store_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"partlist/internal/bomconverter/internal/layout"
	"partlist/internal/bomconverter/internal/store"
	"partlist/internal/platform/db"
	"partlist/internal/platform/migrate"
	"partlist/migrations"
)

// newRepo connects to TEST_DB_URL (a disposable Postgres) and applies all migrations.
// Skipped when the variable is unset so `go test ./...` works without Docker.
func newRepo(t *testing.T) *store.Repository {
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
	if err := conn.Exec("TRUNCATE bom_mappings").Error; err != nil {
		t.Fatal(err)
	}
	return store.New(conn)
}

func mapping(column int) layout.Mapping {
	return layout.Mapping{HeaderRow: 3, Fields: map[string]layout.Source{
		layout.FieldPartNumber:  {Kind: layout.KindColumn, Column: column},
		layout.FieldProjectName: {Kind: layout.KindCell, Row: 0, Column: 1},
		layout.FieldPcbRef:      {Kind: layout.KindText, Text: "PCB-1"},
	}}
}

func TestFindMissingIsNotAnError(t *testing.T) {
	_, ok, err := newRepo(t).Find(context.Background(), "nope")
	if err != nil || ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
}

func TestSaveRoundTripsTheMapping(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	if err := repo.Save(ctx, "fp", mapping(2), "alice"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := repo.Find(ctx, "fp")
	if err != nil || !ok || !reflect.DeepEqual(got, mapping(2)) {
		t.Fatalf("got = %+v, ok = %v, err = %v", got, ok, err)
	}
}

func TestSaveReplacesTheEarlierMappingForEveryone(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	if err := repo.Save(ctx, "fp", mapping(2), "alice"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, "fp", mapping(5), "bob"); err != nil {
		t.Fatal(err)
	}
	got, _, err := repo.Find(ctx, "fp")
	if err != nil || got.Fields[layout.FieldPartNumber].Column != 5 {
		t.Fatalf("got = %+v, err = %v", got, err)
	}
}
