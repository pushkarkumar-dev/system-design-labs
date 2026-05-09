package cdn

import (
	"net/http"
	"time"
)

const (
	// L1MaxSize is the hot in-memory cache capacity.
	L1MaxSize = 100
	// L2MaxSize is the warm in-memory cache capacity.
	L2MaxSize = 10_000
	// MaxAdaptiveTTLMultiplier is the maximum TTL doubling factor for AdaptiveTTL.
	MaxAdaptiveTTLMultiplier = 4
)

// TieredProxy is a v2 two-level cache edge node.
//
// Cache hierarchy:
//   - L1 (hot, 100 entries): served first; evictions are demoted to L2.
//   - L2 (warm, 10,000 entries): promoted to L1 on access.
//   - Origin: fetched on complete miss; result is inserted into L1.
//
// Additional v2 features:
//   - PrefetchHints: Link: </url>; rel=prefetch headers trigger background warming.
//   - AdaptiveTTL: entries with high access counts get their TTL doubled on re-cache
//     (up to MaxAdaptiveTTLMultiplier × original).
//   - CacheStats: atomic counters for all tier events.
type TieredProxy struct {
	l1       *Cache
	l2       *Cache
	base     *EdgeProxy
	stats    *CacheStats
	prefetch *Prefetcher
}

// NewTieredProxy creates a TieredProxy backed by the given origin URL.
func NewTieredProxy(origin string, maxBytes int64) *TieredProxy {
	l1 := NewCache(L1MaxSize)
	l2 := NewCache(L2MaxSize)
	base := NewEdgeProxy(origin, 1, maxBytes) // base cache unused; tiers handle storage
	stats := &CacheStats{}
	tp := &TieredProxy{
		l1:    l1,
		l2:    l2,
		base:  base,
		stats: stats,
	}
	tp.prefetch = NewPrefetcher(tp)
	return tp
}

// ServeHTTP implements http.Handler for the v2 TieredProxy.
func (tp *TieredProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := cacheKey(r)

	// L1 lookup.
	if entry, ok := tp.l1.Get(key); ok && !entry.IsExpired() {
		entry.AccessCount++
		tp.stats.L1Hits.Add(1)
		serveFromCache(w, entry, "HIT-L1")
		return
	}

	// L2 lookup.
	if entry, ok := tp.l2.Get(key); ok && !entry.IsExpired() {
		entry.AccessCount++
		tp.stats.L2Hits.Add(1)
		// Promote to L1; L1 eviction demotes to L2.
		tp.insertL1(key, entry)
		serveFromCache(w, entry, "HIT-L2")
		return
	}

	// Miss: fetch from origin.
	tp.stats.Misses.Add(1)
	entry, err := tp.fetchAndCache(r, key)
	if err != nil {
		http.Error(w, "origin error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Check for prefetch hints in the response.
	go tp.prefetch.ProcessLinkHeader(entry.Headers.Get("Link"), tp.base.origin)

	w.Header().Set("X-Cache", "MISS")
	for k, vals := range entry.Headers {
		for _, v := range vals {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(entry.StatusCode)
	_, _ = w.Write(entry.Body)
}

// fetchAndCache fetches from origin and inserts the result into L1.
func (tp *TieredProxy) fetchAndCache(r *http.Request, key string) (*CacheEntry, error) {
	entry, err := tp.base.fetchFromOrigin(r, key)
	if err != nil {
		return nil, err
	}

	// Apply AdaptiveTTL: if the entry exists in L2 with high access count,
	// double its TTL (up to the max multiplier).
	if existing, ok := tp.l2.Get(key); ok {
		entry.AccessCount = existing.AccessCount
		if existing.AccessCount >= 3 {
			multiplied := entry.OriginalTTL * time.Duration(minInt64(existing.AccessCount/3+1, MaxAdaptiveTTLMultiplier))
			entry.ExpiresAt = entry.CachedAt.Add(multiplied)
		}
	}

	tp.stats.BytesCached.Add(int64(len(entry.Body)))
	tp.insertL1(key, entry)
	return entry, nil
}

// insertL1 inserts an entry into L1. If L1 is full, the evicted entry is
// demoted to L2 (not discarded).
func (tp *TieredProxy) insertL1(key string, entry *CacheEntry) {
	evictedKey, evictedEntry := tp.l1.Set(key, entry)
	if evictedKey != "" && evictedEntry != nil {
		// Demote evicted L1 entry to L2 rather than dropping it.
		tp.stats.EvictionCount.Add(1)
		tp.l2.Set(evictedKey, evictedEntry)
	}
}

// WarmURL fetches the given URL into the cache without a client request.
// Used by the prefetcher to warm entries from Link: rel=prefetch hints.
func (tp *TieredProxy) WarmURL(rawURL string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	key := cacheKey(req)
	_, err = tp.fetchAndCache(req, key)
	return err
}

// Stats returns the live CacheStats pointer.
func (tp *TieredProxy) Stats() *CacheStats {
	return tp.stats
}

// L1 returns the L1 cache (for testing).
func (tp *TieredProxy) L1() *Cache {
	return tp.l1
}

// L2 returns the L2 cache (for testing).
func (tp *TieredProxy) L2() *Cache {
	return tp.l2
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
