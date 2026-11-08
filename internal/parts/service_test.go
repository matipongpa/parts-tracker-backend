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

	pnRows      []repo.Row
	lastPN      string
	lastExact   bool
	lastPNLimit int
}

func (f *fakeStore) ByPartNumber(_ context.Context, pn string, exact bool, limit int) ([]repo.Row, error) {
	f.lastPN, f.lastExact, f.lastPNLimit = pn, exact, limit
	return f.pnRows, nil
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

func TestUsageRejectsBlankPartNumber(t *testing.T) {
	svc := &service{store: &fakeStore{}}
	for _, pn := range []string{"", "   ", "\t\n"} {
		if _, err := svc.Usage(context.Background(), pn, false); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Usage(%q) err = %v, want ErrInvalidInput", pn, err)
		}
	}
}

func TestUsageGroupsRowsByProject(t *testing.T) {
	store := &fakeStore{pnRows: []repo.Row{
		{ID: 1, PartNumber: "ABC", ProjectName: "A", PcbRef: "PCB-1"},
		{ID: 2, PartNumber: "ABC", ProjectName: "A", PcbRef: "PCB-2"},
		{ID: 3, PartNumber: "ABC", ProjectName: "B", PcbRef: "PCB-9"},
	}}
	svc := &service{store: store}

	res, err := svc.Usage(context.Background(), "  ABC ", true)
	if err != nil {
		t.Fatal(err)
	}
	if store.lastPN != "ABC" || !store.lastExact || store.lastPNLimit != usageMaxRows+1 {
		t.Errorf("store called with (%q, %v, %d)", store.lastPN, store.lastExact, store.lastPNLimit)
	}
	if res.Matches != 3 || res.Truncated || len(res.Projects) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if got := res.Projects[0]; got.ProjectName != "A" || len(got.Parts) != 2 || got.Parts[1].PcbRef != "PCB-2" {
		t.Errorf("project A = %+v", got)
	}
	if got := res.Projects[1]; got.ProjectName != "B" || len(got.Parts) != 1 || got.Parts[0].ID != "3" {
		t.Errorf("project B = %+v", got)
	}
}

func TestUsageWithNoMatchesReturnsEmptyProjects(t *testing.T) {
	svc := &service{store: &fakeStore{}}
	res, err := svc.Usage(context.Background(), "NOPE", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Projects == nil || len(res.Projects) != 0 || res.Matches != 0 {
		t.Errorf("result = %+v, want empty non-nil projects", res)
	}
}

func TestUsageTruncatesAtCap(t *testing.T) {
	rows := make([]repo.Row, usageMaxRows+1)
	for i := range rows {
		rows[i] = repo.Row{ID: uint64(i + 1), ProjectName: "A"}
	}
	svc := &service{store: &fakeStore{pnRows: rows}}

	res, err := svc.Usage(context.Background(), "ABC", false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || res.Matches != usageMaxRows {
		t.Errorf("truncated=%v matches=%d, want true/%d", res.Truncated, res.Matches, usageMaxRows)
	}
}
