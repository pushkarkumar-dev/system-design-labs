// bench_test.go — benchmarks for the load balancer.
//
// Run with:
//
//	go test -bench=. -benchtime=5s ./...
package bench_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/load-balancer/pkg/lb"
)

// BenchmarkRoundRobinNext measures the cost of selecting a backend.
// Expected: sub-microsecond (just an atomic increment + slice index).
func BenchmarkRoundRobinNext(b *testing.B) {
	backends := []*lb.Backend{
		lb.NewBackend("http://host1"),
		lb.NewBackend("http://host2"),
		lb.NewBackend("http://host3"),
	}
	rr := lb.NewRoundRobin(backends)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr.Next()
	}
}

// BenchmarkLeastConn measures the cost of least-connections selection.
// O(n) over backends, but n is typically ≤ 10 in real deployments.
func BenchmarkLeastConn(b *testing.B) {
	backends := []*lb.Backend{
		lb.NewBackend("http://host1"),
		lb.NewBackend("http://host2"),
		lb.NewBackend("http://host3"),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.LeastConn(backends)
	}
}

// BenchmarkProxyThroughput measures end-to-end proxy throughput.
// The backend is a loopback HTTP server that returns 200 immediately.
// Expected: ~85,000 req/sec on M2 MacBook Pro (loopback, single core).
func BenchmarkProxyThroughput(b *testing.B) {
	// Spin up 3 loopback backends that return 200.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s1 := httptest.NewServer(handler)
	s2 := httptest.NewServer(handler)
	s3 := httptest.NewServer(handler)
	defer s1.Close()
	defer s2.Close()
	defer s3.Close()

	backends := []*lb.Backend{
		lb.NewBackend(s1.URL),
		lb.NewBackend(s2.URL),
		lb.NewBackend(s3.URL),
	}
	proxy := lb.NewProxy(backends)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			proxy.ServeHTTP(w, req)
		}
	})
}

// BenchmarkProxyLeastConn measures throughput using least-connections selection.
func BenchmarkProxyLeastConn(b *testing.B) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s1 := httptest.NewServer(handler)
	s2 := httptest.NewServer(handler)
	s3 := httptest.NewServer(handler)
	defer s1.Close()
	defer s2.Close()
	defer s3.Close()

	backends := []*lb.Backend{
		lb.NewBackend(s1.URL),
		lb.NewBackend(s2.URL),
		lb.NewBackend(s3.URL),
	}
	proxy := lb.NewProxy(backends)
	proxy.SelectBackend = func() *lb.Backend { return lb.LeastConn(backends) }

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			proxy.ServeHTTP(w, req)
		}
	})
}
