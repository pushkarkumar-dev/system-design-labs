// Package cdn implements a CDN edge node in three stages.
//
// v0: HTTP reverse proxy with LRU cache.
//
//	CacheEntry holds body, headers, status code and expiry.
//	Cache is a doubly-linked list + map for O(1) LRU eviction.
//	EdgeProxy checks cache on every request; HIT serves from cache,
//	MISS fetches from origin, stores response, adds X-Cache: HIT/MISS.
//	Cache-Control: max-age=N sets TTL; no-store bypasses the cache.
//
// v1: Stale-while-revalidate + request coalescing.
//
//	stale-while-revalidate=N: serve stale immediately, refresh in background.
//	stale-if-error=N: serve stale when origin returns 5xx.
//	Request coalescing: N concurrent misses on the same URL make exactly one
//	origin call; all waiters receive the same CacheEntry via a shared channel.
//	Purge API: DELETE /cache/purge?path= invalidates a single entry.
//	Vary header: separate cache entries per Accept-Encoding (or other fields).
//
// v2: Two-level cache tiers + prefetching.
//
//	L1 (100 entries, hot) + L2 (10,000 entries, warm).
//	L1 miss → check L2; L2 hit promotes to L1.
//	L1 eviction demotes to L2 (not dropped).
//	Link: </url>; rel=prefetch headers trigger background warming.
//	CacheStats: atomic counters for L1 hits, L2 hits, misses, bytes cached.
//	AdaptiveTTL: frequently accessed entries get doubled TTL on re-cache.
package cdn
