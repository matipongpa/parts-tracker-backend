package parts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"

	"golang.org/x/sync/singleflight"
)

// cachedService decorates Search with a cache. Every successful write bumps a version that is
// part of the key, so older entries become unreachable and age out; no key scanning is needed.
type cachedService struct {
	Service
	cache   Cache
	version atomic.Uint64
	flight  singleflight.Group
}

func newCachedService(inner Service, cache Cache) *cachedService {
	return &cachedService{Service: inner, cache: cache}
}

func (c *cachedService) Search(ctx context.Context, q SearchQuery) (SearchResult, error) {
	key := searchKey(c.version.Load(), q)
	if hit, ok := c.cache.Get(key); ok {
		return hit, nil
	}
	// The key carries the version read above, so a write that lands mid-load can only
	// leave a result under a key nobody will ask for again.
	v, err, _ := c.flight.Do(key, func() (interface{}, error) {
		res, err := c.Service.Search(ctx, q)
		if err != nil {
			return nil, err
		}
		c.cache.Set(key, res)
		return res, nil
	})
	if err != nil {
		return SearchResult{}, err
	}
	res, ok := v.(SearchResult)
	if !ok {
		return SearchResult{}, fmt.Errorf("parts: unexpected cached type %T", v)
	}
	return res, nil
}

func (c *cachedService) Create(ctx context.Context, p NewPart) (Part, error) {
	part, err := c.Service.Create(ctx, p)
	if err == nil {
		c.version.Add(1)
	}
	return part, err
}

func (c *cachedService) Update(ctx context.Context, id string, p PartPatch) (Part, error) {
	part, err := c.Service.Update(ctx, id, p)
	if err == nil {
		c.version.Add(1)
	}
	return part, err
}

func (c *cachedService) Delete(ctx context.Context, id string) error {
	err := c.Service.Delete(ctx, id)
	if err == nil {
		c.version.Add(1)
	}
	return err
}

// BulkCreate bumps once per call, not once per row.
func (c *cachedService) BulkCreate(ctx context.Context, ps []NewPart) (int, error) {
	n, err := c.Service.BulkCreate(ctx, ps)
	if err == nil && n > 0 {
		c.version.Add(1)
	}
	return n, err
}

// searchKey is search:v{version}:{sha256(normalized query)}. Filters are trimmed and lowercased
// (matching is case-insensitive) and the page is normalized so equivalent queries share a key.
func searchKey(version uint64, q SearchQuery) string {
	offset, limit := normalizePage(q.Page, q.PageSize)
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	raw := fmt.Sprintf("%q|%q|%q|%q|%q|%d|%d",
		norm(q.PcbRef), norm(q.PartNumber), norm(q.Value), norm(q.Description), norm(q.Type), offset, limit)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("search:v%d:%s", version, hex.EncodeToString(sum[:]))
}
