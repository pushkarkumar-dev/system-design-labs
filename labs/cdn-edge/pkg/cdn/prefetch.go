package cdn

import (
	"strings"
)

// Prefetcher processes Link: <url>; rel=prefetch response headers and
// asynchronously warms the cache for linked URLs.
//
// Example header:
//
//	Link: </next-page>; rel=prefetch, </images/hero.jpg>; rel=prefetch
//
// When the edge node receives this header in an origin response, the
// Prefetcher extracts the URLs and calls WarmURL for each one in a
// background goroutine. This warms the cache before any client requests
// the linked resources, reducing MISS latency for predictable access patterns.
type Prefetcher struct {
	warmer interface {
		WarmURL(rawURL string) error
	}
}

// NewPrefetcher creates a Prefetcher backed by the given cache warmer.
func NewPrefetcher(warmer interface{ WarmURL(rawURL string) error }) *Prefetcher {
	return &Prefetcher{warmer: warmer}
}

// ProcessLinkHeader parses the Link header value and warms any rel=prefetch URLs.
// The base parameter is the origin URL prefix (e.g., "http://localhost:9000")
// used to resolve absolute-path references like "/next-page".
//
// Each URL is fetched in its own goroutine so that slow origins do not block
// the caller.
func (pf *Prefetcher) ProcessLinkHeader(linkHeader, base string) {
	if linkHeader == "" {
		return
	}
	for _, link := range strings.Split(linkHeader, ",") {
		link = strings.TrimSpace(link)
		if !strings.Contains(strings.ToLower(link), "rel=prefetch") {
			continue
		}
		rawURL := extractLinkURL(link, base)
		if rawURL == "" {
			continue
		}
		go func(u string) {
			_ = pf.warmer.WarmURL(u)
		}(rawURL)
	}
}

// extractLinkURL extracts the URL from a single Link field value.
// Format: <url>; param=value; ...
// Absolute-path references (starting with /) are resolved against base.
func extractLinkURL(linkField, base string) string {
	start := strings.Index(linkField, "<")
	end := strings.Index(linkField, ">")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	rawURL := linkField[start+1 : end]
	if rawURL == "" {
		return ""
	}
	// Resolve absolute-path references.
	if strings.HasPrefix(rawURL, "/") {
		return strings.TrimRight(base, "/") + rawURL
	}
	return rawURL
}
