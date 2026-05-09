package cdn_bench

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/cdn-edge/pkg/cdn"
)

// BenchmarkL1Hit measures the throughput of L1 cache hits in the TieredProxy.
// This is the hot path: map lookup + mutex + response copy.
func BenchmarkL1Hit(b *testing.B) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		io.WriteString(w, "cached content")
	}))
	defer origin.Close()

	proxy := cdn.NewTieredProxy(origin.URL, cdn.DefaultMaxBytes)

	// Warm the cache.
	req := httptest.NewRequest(http.MethodGet, "/bench", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/bench", nil)
			w := httptest.NewRecorder()
			proxy.ServeHTTP(w, req)
		}
	})
}

// BenchmarkCacheMiss measures throughput when each request is a cache miss
// (unique URL) and the origin is a loopback HTTP server.
func BenchmarkCacheMiss(b *testing.B) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		io.WriteString(w, "fresh content")
	}))
	defer origin.Close()

	proxy := cdn.NewTieredProxy(origin.URL, cdn.DefaultMaxBytes)

	var counter atomic.Int64
	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := counter.Add(1)
			path := fmt.Sprintf("/miss/%d", n)
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			proxy.ServeHTTP(w, req)
		}
	})
}

// BenchmarkLRUEviction measures the eviction throughput on the doubly-linked list.
func BenchmarkLRUEviction(b *testing.B) {
	cache := cdn.NewCache(100) // small cache to force constant eviction

	entry := &cdn.CacheEntry{Body: []byte("bench")}
	var counter atomic.Int64

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		n := counter.Add(1)
		key := fmt.Sprintf("key-%d", n)
		cache.Set(key, entry)
	}
}

// BenchmarkRequestCoalescing measures the coalescing throughput under contention.
func BenchmarkRequestCoalescing(b *testing.B) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		io.WriteString(w, "coalesced")
	}))
	defer origin.Close()

	cp := cdn.NewCoalescingProxy(origin.URL, 10, cdn.DefaultMaxBytes)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/coalesce", nil)
			w := httptest.NewRecorder()
			cp.ServeHTTP(w, req)
		}
	})
}
