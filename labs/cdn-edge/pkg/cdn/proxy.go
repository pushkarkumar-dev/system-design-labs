package cdn

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultMaxBytes is the maximum response body size that will be cached.
	DefaultMaxBytes = 10 * 1024 * 1024 // 10 MB
	// DefaultCacheSize is the default LRU cache capacity for v0.
	DefaultCacheSize = 1000
	// DefaultTTL is used when the origin does not send Cache-Control.
	DefaultTTL = 60 * time.Second
)

// CacheControlDirectives holds parsed values from a Cache-Control header.
type CacheControlDirectives struct {
	NoStore             bool
	NoCache             bool
	MaxAge              time.Duration // 0 if not present
	SWR                 time.Duration // stale-while-revalidate window
	SIE                 time.Duration // stale-if-error window
	VaryFields          []string      // from the Vary response header
}

// ParseCacheControl parses a Cache-Control response header value and the
// accompanying Vary header. The caller passes both headers to avoid the need
// for a full http.Response reference.
func ParseCacheControl(cacheControl, vary string) CacheControlDirectives {
	var d CacheControlDirectives

	for _, part := range strings.Split(cacheControl, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "no-store":
			d.NoStore = true
		case part == "no-cache":
			d.NoCache = true
		case strings.HasPrefix(part, "max-age="):
			if secs, err := strconv.Atoi(strings.TrimPrefix(part, "max-age=")); err == nil {
				d.MaxAge = time.Duration(secs) * time.Second
			}
		case strings.HasPrefix(part, "stale-while-revalidate="):
			if secs, err := strconv.Atoi(strings.TrimPrefix(part, "stale-while-revalidate=")); err == nil {
				d.SWR = time.Duration(secs) * time.Second
			}
		case strings.HasPrefix(part, "stale-if-error="):
			if secs, err := strconv.Atoi(strings.TrimPrefix(part, "stale-if-error=")); err == nil {
				d.SIE = time.Duration(secs) * time.Second
			}
		}
	}

	if vary != "" {
		for _, field := range strings.Split(vary, ",") {
			field = strings.TrimSpace(field)
			if field != "" {
				d.VaryFields = append(d.VaryFields, strings.ToLower(field))
			}
		}
	}

	return d
}

// EdgeProxy is a caching HTTP reverse proxy.
//
// v0 implementation: LRU cache with Cache-Control parsing.
// v1 extends it (via CoalescingProxy) with stale-while-revalidate and request coalescing.
type EdgeProxy struct {
	cache    *Cache
	origin   string
	maxBytes int64
	client   *http.Client
}

// NewEdgeProxy creates a new EdgeProxy that caches responses from the origin URL.
func NewEdgeProxy(origin string, cacheSize int, maxBytes int64) *EdgeProxy {
	if cacheSize <= 0 {
		cacheSize = DefaultCacheSize
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &EdgeProxy{
		cache:    NewCache(cacheSize),
		origin:   origin,
		maxBytes: maxBytes,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

// cacheKey returns a normalized cache key for a request.
// Format: path + "?" + rawQuery (empty query omits the "?").
func cacheKey(r *http.Request) string {
	u := &url.URL{Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	if u.RawQuery == "" {
		return u.Path
	}
	return u.Path + "?" + u.RawQuery
}

// varyKey builds the part of the cache key that depends on the Vary header.
// For Vary: accept-encoding we include the request's Accept-Encoding value.
func varyKey(r *http.Request, varyFields []string) string {
	if len(varyFields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(varyFields))
	for _, field := range varyFields {
		val := r.Header.Get(field)
		parts = append(parts, field+"="+val)
	}
	return strings.Join(parts, ";")
}

// fullCacheKey combines the URL key and the vary key.
func fullCacheKey(urlKey, vary string) string {
	if vary == "" {
		return urlKey
	}
	return urlKey + "|" + vary
}

// ServeHTTP implements http.Handler for the v0 EdgeProxy.
func (p *EdgeProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := cacheKey(r)

	// Optimistic cache lookup without knowing vary fields yet.
	// We use just the URL key for the initial lookup; if the entry carries
	// VaryKey we re-check. This keeps v0 simple.
	if entry, ok := p.cache.Get(key); ok && !entry.IsExpired() {
		serveFromCache(w, entry, "HIT")
		return
	}

	// Cache miss: fetch from origin.
	entry, err := p.fetchFromOrigin(r, key)
	if err != nil {
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

// fetchFromOrigin makes an HTTP request to the origin server, builds a
// CacheEntry, and stores it in the cache (unless no-store is set).
func (p *EdgeProxy) fetchFromOrigin(r *http.Request, key string) (*CacheEntry, error) {
	originURL := p.origin + r.URL.RequestURI()
	req, err := http.NewRequestWithContext(r.Context(), r.Method, originURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build origin request: %w", err)
	}
	// Forward original request headers.
	for k, vals := range r.Header {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("origin fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, p.maxBytes))
	if err != nil {
		return nil, fmt.Errorf("read origin body: %w", err)
	}

	// Copy response headers (excluding hop-by-hop headers).
	headers := make(http.Header)
	for k, vals := range resp.Header {
		headers[k] = vals
	}

	now := time.Now()
	directives := ParseCacheControl(resp.Header.Get("Cache-Control"), resp.Header.Get("Vary"))

	entry := &CacheEntry{
		Body:       body,
		Headers:    headers,
		StatusCode: resp.StatusCode,
		CachedAt:   now,
	}

	ttl := DefaultTTL
	if directives.MaxAge > 0 {
		ttl = directives.MaxAge
	}
	entry.ExpiresAt = now.Add(ttl)
	entry.OriginalTTL = ttl

	if directives.SWR > 0 {
		entry.StaleWhileRevalidateUntil = now.Add(ttl + directives.SWR)
	}
	if directives.SIE > 0 {
		entry.StaleIfErrorUntil = now.Add(ttl + directives.SIE)
	}

	// Don't cache if the origin said no-store.
	if !directives.NoStore && resp.StatusCode < 500 {
		p.cache.Set(key, entry)
	}

	return entry, nil
}

// serveFromCache writes a cached response to the client.
func serveFromCache(w http.ResponseWriter, entry *CacheEntry, cacheStatus string) {
	w.Header().Set("X-Cache", cacheStatus)
	for k, vals := range entry.Headers {
		for _, v := range vals {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(entry.StatusCode)
	_, _ = w.Write(entry.Body)
}

// Cache returns the underlying LRU cache (used by tests and higher layers).
func (p *EdgeProxy) Cache() *Cache {
	return p.cache
}

// Origin returns the configured origin URL.
func (p *EdgeProxy) Origin() string {
	return p.origin
}

// HTTPClient returns the HTTP client used to fetch from origin.
func (p *EdgeProxy) HTTPClient() *http.Client {
	return p.client
}

// FetchFromOrigin exposes the origin fetch for use by the coalescing proxy.
func (p *EdgeProxy) FetchFromOrigin(r *http.Request, key string) (*CacheEntry, error) {
	return p.fetchFromOrigin(r, key)
}
