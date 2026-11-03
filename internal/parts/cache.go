package parts

import (
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
)

// Cache stores search results by key. Implementations must be safe for concurrent use.
// Cached values are shared between callers and must be treated as read-only.
type Cache interface {
	Get(key string) (SearchResult, bool)
	Set(key string, value SearchResult)
}

// newLRUCache returns a bounded in-process cache whose entries expire after ttl.
func newLRUCache(maxEntries int, ttl time.Duration) Cache {
	return &lruCache{store: lru.NewLRU[string, SearchResult](maxEntries, nil, ttl)}
}

type lruCache struct {
	store *lru.LRU[string, SearchResult]
}

func (c *lruCache) Get(key string) (SearchResult, bool) { return c.store.Get(key) }
func (c *lruCache) Set(key string, value SearchResult)  { c.store.Add(key, value) }
