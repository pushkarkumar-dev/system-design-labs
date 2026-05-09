// bench_test.go — benchmarks for the service discovery registry.
//
// Run from labs/service-discovery/:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
package discovery_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/service-discovery/pkg/discovery"
)

// BenchmarkRegistry_Lookup measures the read-path cost when 100 instances
// are registered. This exercises the RWMutex + slice copy in Lookup.
func BenchmarkRegistry_Lookup(b *testing.B) {
	r := discovery.NewRegistry()
	for i := 0; i < 100; i++ {
		_ = r.Register(discovery.ServiceInstance{
			ID:          fmt.Sprintf("inst-%d", i),
			ServiceName: "bench-svc",
			Host:        fmt.Sprintf("10.0.0.%d", i%256),
			Port:        "8080",
		})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Lookup("bench-svc")
	}
}

// BenchmarkRegistry_Register measures the write-path cost: exclusive lock +
// map insertion + byID update.
func BenchmarkRegistry_Register(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		r := discovery.NewRegistry()
		b.StartTimer()
		_ = r.Register(discovery.ServiceInstance{
			ID:          "bench-inst",
			ServiceName: "bench-svc",
			Host:        "10.0.0.1",
			Port:        "8080",
		})
	}
}

// BenchmarkServiceClient_Resolve measures the hot path: atomic counter +
// in-process slice index. No lock contention in the common case.
func BenchmarkServiceClient_Resolve(b *testing.B) {
	hr := discovery.NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	for i := 0; i < 5; i++ {
		_ = hr.Register(discovery.ServiceInstance{
			ID:          fmt.Sprintf("inst-%d", i),
			ServiceName: "bench-svc",
			Host:        fmt.Sprintf("10.0.0.%d", i),
			Port:        "8080",
		}, 30*time.Second)
	}

	sc := discovery.NewServiceClient(hr)
	defer sc.Stop()

	// Warm up the cache.
	_, _, _ = sc.Resolve("bench-svc")

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = sc.Resolve("bench-svc")
	}
}

// BenchmarkWatchEventDelivery measures the latency from Register to Watch
// channel delivery. This exercises the notify path: lock, copy slice, send.
func BenchmarkWatchEventDelivery(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		hr := discovery.NewHealthRegistry(30 * time.Second)
		events := hr.Watch("bench-svc")
		b.StartTimer()

		_ = hr.Register(discovery.ServiceInstance{
			ID:          fmt.Sprintf("inst-%d", i),
			ServiceName: "bench-svc",
			Host:        "10.0.0.1",
			Port:        "8080",
		}, 30*time.Second)

		<-events // block until the event arrives
		b.StopTimer()
		hr.Stop()
		b.StartTimer()
	}
}
