package cdn

import (
	"container/list"
	"net/http"
	"sync"
	"time"
)

// CacheEntry holds a single cached HTTP response.
type CacheEntry struct {
	Body       []byte
	Headers    http.Header
	StatusCode int
	ExpiresAt  time.Time
	CachedAt   time.Time
	// StaleWhileRevalidateUntil: serve stale up to this time while background fetch runs.
	StaleWhileRevalidateUntil time.Time
	// StaleIfErrorUntil: serve stale on 5xx up to this time.
	StaleIfErrorUntil time.Time
	// VaryKey: the normalized value of the Vary header field that distinguished this entry.
	VaryKey string
	// AccessCount: how many times this entry has been served (for AdaptiveTTL).
	AccessCount int64
	// OriginalTTL: the base TTL used at cache time (for adaptive doubling).
	OriginalTTL time.Duration
}

// IsExpired returns true if the entry's TTL has passed.
func (e *CacheEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// IsStaleWhileRevalidate returns true if the entry is expired but within
// the stale-while-revalidate window. The caller should serve the stale entry
// immediately and trigger a background revalidation.
func (e *CacheEntry) IsStaleWhileRevalidate() bool {
	now := time.Now()
	return now.After(e.ExpiresAt) && now.Before(e.StaleWhileRevalidateUntil)
}

// IsStaleIfError returns true if the entry is expired but within the
// stale-if-error window. The caller should serve this on origin 5xx.
func (e *CacheEntry) IsStaleIfError() bool {
	now := time.Now()
	return now.After(e.ExpiresAt) && now.Before(e.StaleIfErrorUntil)
}

// lruItem is stored in the list.Element.Value field.
type lruItem struct {
	key   string
	entry *CacheEntry
}

// Cache is a thread-safe LRU cache backed by a doubly-linked list and a map.
//
// All operations are O(1):
//   - Get: map lookup + list.MoveToFront
//   - Set: map insert + list.PushFront; if over capacity, list.Back removal + map delete
//   - Invalidate: map lookup + list.Remove + map delete
type Cache struct {
	mu      sync.Mutex
	items   map[string]*list.Element // key → list element
	order   *list.List               // front = most recently used
	maxSize int
}

// NewCache creates a new LRU cache with the given capacity.
func NewCache(maxSize int) *Cache {
	return &Cache{
		items:   make(map[string]*list.Element, maxSize),
		order:   list.New(),
		maxSize: maxSize,
	}
}

// Get retrieves a cache entry by key. On a hit, the entry is moved to the
// front of the LRU list (most recently used position).
func (c *Cache) Get(key string) (*CacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*lruItem).entry, true
}

// Set inserts or updates a cache entry. If the cache is at capacity, the
// least-recently-used entry is evicted. The evicted entry is returned so
// callers (e.g., the tiered cache) can demote it rather than discard it.
func (c *Cache) Set(key string, entry *CacheEntry) (evictedKey string, evictedEntry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		// Update existing entry in place, move to front.
		elem.Value.(*lruItem).entry = entry
		c.order.MoveToFront(elem)
		return "", nil
	}

	// Evict LRU tail if at capacity.
	if c.order.Len() >= c.maxSize {
		tail := c.order.Back()
		if tail != nil {
			item := tail.Value.(*lruItem)
			evictedKey = item.key
			evictedEntry = item.entry
			delete(c.items, item.key)
			c.order.Remove(tail)
		}
	}

	elem := c.order.PushFront(&lruItem{key: key, entry: entry})
	c.items[key] = elem
	return evictedKey, evictedEntry
}

// Invalidate removes an entry from the cache. It is a no-op if the key is
// not present.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		delete(c.items, key)
		c.order.Remove(elem)
	}
}

// Len returns the current number of entries in the cache.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}
