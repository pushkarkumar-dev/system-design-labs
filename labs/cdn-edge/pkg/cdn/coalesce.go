package cdn

import (
	"fmt"
	"net/http"
	"sync"
)

// inflightReq represents a single in-flight origin fetch for a given cache key.
// The first goroutine to discover a miss creates an inflightReq and starts the
// fetch. All subsequent goroutines for the same key wait on done; once the
// first goroutine completes, it broadcasts by closing done.
type inflightReq struct {
	done  chan struct{}
	entry *CacheEntry
	err   error
}

// CoalescingProxy wraps EdgeProxy with:
//   - Request coalescing: N concurrent misses on the same key trigger exactly
//     one origin fetch; all waiters receive the same CacheEntry.
//   - Stale-while-revalidate: expired entries within the SWR window are served
//     immediately while a background goroutine revalidates.
//   - Stale-if-error: origin 5xx responses fall back to a stale entry if one
//     exists and is within the stale-if-error window.
//   - Purge API: DELETE /cache/purge?path= invalidates a single entry.
//   - Vary: separate cache entries per Vary field value.
type CoalescingProxy struct {
	base *EdgeProxy

	mu       sync.Mutex
	inflight map[string]*inflightReq // key → in-progress fetch
}

// NewCoalescingProxy creates a CoalescingProxy backed by the given EdgeProxy.
func NewCoalescingProxy(origin string, cacheSize int, maxBytes int64) *CoalescingProxy {
	return &CoalescingProxy{
		base:     NewEdgeProxy(origin, cacheSize, maxBytes),
		inflight: make(map[string]*inflightReq),
	}
}

// ServeHTTP implements http.Handler for the v1 CoalescingProxy.
// It registers /cache/purge separately; see RegisterRoutes.
func (cp *CoalescingProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Route purge requests.
	if r.Method == http.MethodDelete && r.URL.Path == "/cache/purge" {
		cp.handlePurge(w, r)
		return
	}

	key := cacheKey(r)

	// Fast path: check cache with stale-while-revalidate logic.
	if entry, ok := cp.base.cache.Get(key); ok {
		if !entry.IsExpired() {
			serveFromCache(w, entry, "HIT")
			return
		}
		if entry.IsStaleWhileRevalidate() {
			// Serve stale immediately; revalidate in background.
			go cp.revalidate(r, key)
			serveFromCache(w, entry, "STALE")
			return
		}
	}

	// Slow path: coalesced origin fetch.
	entry, stale, err := cp.coalescedFetch(r, key)
	if err != nil {
		// Try stale-if-error.
		if stale != nil && stale.IsStaleIfError() {
			serveFromCache(w, stale, "STALE-IF-ERROR")
			return
		}
		http.Error(w, "origin error: "+err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("X-Cache", "MISS")
	for k, vals := range entry.Headers {
		for _, v := range vals {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(entry.StatusCode)
	_, _ = w.Write(entry.Body)
}

// coalescedFetch ensures exactly one origin request is made for a given key,
// even if N goroutines all arrive at a cache miss simultaneously. It returns:
//   - (entry, stale, nil) on a successful fetch
//   - (nil, stale, err) when the origin returns an error; stale may be non-nil
//     if a stale-if-error candidate exists in cache
func (cp *CoalescingProxy) coalescedFetch(r *http.Request, key string) (*CacheEntry, *CacheEntry, error) {
	// Retrieve any stale entry before locking (for stale-if-error fallback).
	var staleEntry *CacheEntry
	if e, ok := cp.base.cache.Get(key); ok {
		staleEntry = e
	}

	cp.mu.Lock()
	// Check again under the lock — another goroutine may have just populated.
	if e, ok := cp.base.cache.Get(key); ok && !e.IsExpired() {
		cp.mu.Unlock()
		return e, nil, nil
	}

	if req, ok := cp.inflight[key]; ok {
		// Another goroutine is already fetching this key. Wait for it.
		cp.mu.Unlock()
		<-req.done
		if req.err != nil {
			return nil, staleEntry, req.err
		}
		return req.entry, nil, nil
	}

	// We are the first goroutine for this key. Register the inflight request.
	req := &inflightReq{done: make(chan struct{})}
	cp.inflight[key] = req
	cp.mu.Unlock()

	// Fetch from origin (outside the lock).
	entry, err := cp.base.fetchFromOrigin(r, key)

	// Populate the inflight result and broadcast to waiters.
	cp.mu.Lock()
	req.entry = entry
	req.err = err
	delete(cp.inflight, key)
	cp.mu.Unlock()
	close(req.done)

	if err != nil {
		return nil, staleEntry, fmt.Errorf("origin fetch: %w", err)
	}
	return entry, nil, nil
}

// revalidate fetches a fresh copy of the resource from origin and updates the
// cache. It is called in a background goroutine by the stale-while-revalidate path.
func (cp *CoalescingProxy) revalidate(r *http.Request, key string) {
	// Reuse the origin fetch; the result is stored in the cache by fetchFromOrigin.
	_, _ = cp.base.fetchFromOrigin(r, key)
}

// handlePurge handles DELETE /cache/purge?path=<path>.
// It invalidates the given path from the cache.
func (cp *CoalescingProxy) handlePurge(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing ?path= query parameter", http.StatusBadRequest)
		return
	}
	cp.base.cache.Invalidate(path)
	w.WriteHeader(http.StatusNoContent)
}

// Invalidate explicitly purges the given key from the underlying cache.
func (cp *CoalescingProxy) Invalidate(key string) {
	cp.base.cache.Invalidate(key)
}

// Cache returns the underlying LRU cache.
func (cp *CoalescingProxy) Cache() *Cache {
	return cp.base.cache
}

// BaseProxy returns the underlying EdgeProxy (for testing).
func (cp *CoalescingProxy) BaseProxy() *EdgeProxy {
	return cp.base
}
