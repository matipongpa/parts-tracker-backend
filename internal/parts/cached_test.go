package parts

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingService struct {
	Service
	searches atomic.Int64
	gate     chan struct{}
	failNext bool
}

func (s *countingService) Search(context.Context, SearchQuery) (SearchResult, error) {
	s.searches.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	if s.failNext {
		return SearchResult{}, errors.New("db down")
	}
	return SearchResult{Count: s.searches.Load()}, nil
}
func (s *countingService) Create(context.Context, NewPart) (Part, error) { return Part{}, nil }
func (s *countingService) Update(context.Context, string, PartPatch) (Part, error) {
	return Part{}, ErrNotFound
}
func (s *countingService) Delete(context.Context, string) error { return nil }
func (s *countingService) BulkCreate(_ context.Context, ps []NewPart) (int, error) {
	return len(ps), nil
}

func newCached(inner *countingService) *cachedService {
	return newCachedService(inner, newLRUCache(10, time.Minute))
}

func TestSearchKeyNormalizesEquivalentQueries(t *testing.T) {
	a := searchKey(1, SearchQuery{Type: " Cap ", PageSize: 0})
	b := searchKey(1, SearchQuery{Type: "cap", PageSize: 10})
	if a != b {
		t.Errorf("equivalent queries got different keys")
	}
	if a == searchKey(2, SearchQuery{Type: "cap"}) {
		t.Errorf("version must change the key")
	}
	if searchKey(1, SearchQuery{PcbRef: "a", PartNumber: ""}) == searchKey(1, SearchQuery{PcbRef: "", PartNumber: "a"}) {
		t.Errorf("different fields must not collide")
	}
}

func TestSearchIsCachedUntilWrite(t *testing.T) {
	inner := &countingService{}
	c := newCached(inner)
	ctx := context.Background()
	q := SearchQuery{Type: "cap"}

	c.Search(ctx, q)
	c.Search(ctx, q)
	if got := inner.searches.Load(); got != 1 {
		t.Fatalf("searches = %d, want 1", got)
	}

	if _, err := c.Create(ctx, NewPart{}); err != nil {
		t.Fatal(err)
	}
	c.Search(ctx, q)
	if got := inner.searches.Load(); got != 2 {
		t.Fatalf("searches after write = %d, want 2", got)
	}
}

func TestFailedWriteDoesNotInvalidate(t *testing.T) {
	inner := &countingService{}
	c := newCached(inner)
	ctx := context.Background()
	c.Search(ctx, SearchQuery{})
	c.Update(ctx, "1", PartPatch{}) // returns ErrNotFound
	c.Search(ctx, SearchQuery{})
	if got := inner.searches.Load(); got != 1 {
		t.Fatalf("searches = %d, want 1", got)
	}
}

func TestBulkCreateBumpsOnceAndEmptyDoesNot(t *testing.T) {
	c := newCached(&countingService{})
	ctx := context.Background()
	c.BulkCreate(ctx, nil)
	if v := c.version.Load(); v != 0 {
		t.Errorf("version after empty bulk = %d", v)
	}
	c.BulkCreate(ctx, make([]NewPart, 50))
	if v := c.version.Load(); v != 1 {
		t.Errorf("version after bulk = %d, want 1", v)
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	inner := &countingService{failNext: true}
	c := newCached(inner)
	if _, err := c.Search(context.Background(), SearchQuery{}); err == nil {
		t.Fatal("expected error")
	}
	inner.failNext = false
	if _, err := c.Search(context.Background(), SearchQuery{}); err != nil {
		t.Fatal(err)
	}
	if got := inner.searches.Load(); got != 2 {
		t.Fatalf("searches = %d, want 2", got)
	}
}

func TestConcurrentMissesShareOneLoad(t *testing.T) {
	inner := &countingService{gate: make(chan struct{})}
	c := newCached(inner)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Search(context.Background(), SearchQuery{}) }()
	}
	time.Sleep(100 * time.Millisecond)
	close(inner.gate)
	wg.Wait()
	if got := inner.searches.Load(); got != 1 {
		t.Fatalf("searches = %d, want 1 (singleflight)", got)
	}
}
