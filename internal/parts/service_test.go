package parts

import (
	"context"
	"errors"
	"testing"

	"partlist/internal/parts/internal/repo"
)

type fakeStore struct {
	lastFilter repo.Filter
	lastID     uint64
	lastPatch  repo.Patch
	bulkRows   []repo.Row
	getErr     error
}

func (f *fakeStore) Get(_ context.Context, id uint64) (repo.Row, error) {
	f.lastID = id
	return repo.Row{ID: id}, f.getErr
}
func (f *fakeStore) Search(_ context.Context, fl repo.Filter) ([]repo.Row, int64, error) {
	f.lastFilter = fl
	return []repo.Row{{ID: 1}}, 42, nil
}
func (f *fakeStore) Create(_ context.Context, r repo.Row) (repo.Row, error) { r.ID = 7; return r, nil }
func (f *fakeStore) Update(_ context.Context, id uint64, p repo.Patch) (repo.Row, error) {
	f.lastID, f.lastPatch = id, p
	return repo.Row{ID: id}, nil
}
func (f *fakeStore) Delete(_ context.Context, id uint64) error { f.lastID = id; return nil }
func (f *fakeStore) BulkCreate(_ context.Context, rows []repo.Row) (int, error) {
	f.bulkRows = rows
	return len(rows), nil
}

func validPart() NewPart {
	return NewPart{PartNumber: "PN", PcbRef: "C1", Fit: "FIT", Manufacturer: "M", Type: "Capacitor", ProjectName: "P", Value: "1u"}
}

func TestNormalizePage(t *testing.T) {
	cases := []struct{ page, size, wantOffset, wantLimit int }{
		{0, 0, 0, 10},
		{2, 20, 40, 20},
		{1, 1000, 100, 100},
		{-3, 5, 0, 5},
	}
	for _, tc := range cases {
		offset, limit := normalizePage(tc.page, tc.size)
		if offset != tc.wantOffset || limit != tc.wantLimit {
			t.Errorf("normalizePage(%d,%d) = (%d,%d), want (%d,%d)", tc.page, tc.size, offset, limit, tc.wantOffset, tc.wantLimit)
		}
	}
}

func TestSearchPassesNormalizedPageAndCount(t *testing.T) {
	store := &fakeStore{}
	svc := &service{store: store}
	res, err := svc.Search(context.Background(), SearchQuery{Type: "cap", Page: 2, PageSize: 500})
	if err != nil {
		t.Fatal(err)
	}
	if store.lastFilter.Offset != 200 || store.lastFilter.Limit != 100 || store.lastFilter.Type != "cap" {
		t.Errorf("filter = %+v", store.lastFilter)
	}
	if res.Count != 42 || len(res.List) != 1 || res.List[0].ID != "1" {
		t.Errorf("result = %+v", res)
	}
}

func TestIDValidation(t *testing.T) {
	svc := &service{store: &fakeStore{}}
	for _, id := range []string{"", "abc", "0", "-1", "1; DROP"} {
		if _, err := svc.Get(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) err = %v, want ErrNotFound", id, err)
		}
		if err := svc.Delete(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Delete(%q) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestGetMapsRepoNotFound(t *testing.T) {
	svc := &service{store: &fakeStore{getErr: repo.ErrNotFound}}
	if _, err := svc.Get(context.Background(), "5"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateRequiresFields(t *testing.T) {
	svc := &service{store: &fakeStore{}}
	if _, err := svc.Create(context.Background(), NewPart{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
	p, err := svc.Create(context.Background(), validPart())
	if err != nil || p.ID != "7" {
		t.Fatalf("p = %+v, err = %v", p, err)
	}
}

func TestUpdateKeepsEmptyStringPatch(t *testing.T) {
	store := &fakeStore{}
	svc := &service{store: store}
	empty := ""
	if _, err := svc.Update(context.Background(), "3", PartPatch{Description: &empty}); err != nil {
		t.Fatal(err)
	}
	if store.lastPatch.Description == nil || *store.lastPatch.Description != "" || store.lastID != 3 {
		t.Errorf("patch = %+v id = %d", store.lastPatch, store.lastID)
	}
}

func TestBulkCreateValidatesAllBeforeStoring(t *testing.T) {
	store := &fakeStore{}
	svc := &service{store: store}
	_, err := svc.BulkCreate(context.Background(), []NewPart{validPart(), {}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
	if store.bulkRows != nil {
		t.Error("store was called despite invalid input")
	}
	n, err := svc.BulkCreate(context.Background(), []NewPart{validPart(), validPart()})
	if err != nil || n != 2 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
}
