package bench_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/faas-runtime/pkg/faas"
)

// BenchmarkWarmInvocation measures the throughput of warm invocations —
// the common case where an instance is already in the pool.
func BenchmarkWarmInvocation(b *testing.B) {
	rt := faas.NewRuntimeWithPool(3)
	rt.Register("noop", func(ctx context.Context, req faas.Request) faas.Response {
		return faas.Response{StatusCode: http.StatusOK, Body: []byte("ok")}
	}, 5*time.Second)

	// Warm up: one cold start to populate the pool.
	rt.Invoke(context.Background(), "noop", faas.Request{})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rt.Invoke(context.Background(), "noop", faas.Request{})
		}
	})
}

// BenchmarkColdStart measures the latency of a cold start (50ms simulated).
func BenchmarkColdStart(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rt := faas.NewRuntimeWithPool(0) // maxWarm=0: every call is a cold start
		rt.Register("fn", func(ctx context.Context, req faas.Request) faas.Response {
			return faas.Response{StatusCode: http.StatusOK}
		}, 5*time.Second)
		rt.Invoke(context.Background(), "fn", faas.Request{})
	}
}

// BenchmarkSnapshotRestore measures the latency of restoring from a snapshot (5ms).
func BenchmarkSnapshotRestore(b *testing.B) {
	store := faas.NewSnapshotStore()
	// Pre-populate the snapshot.
	store.Save("fn", []byte("initialized"))

	pool := faas.NewInstancePool(0) // no warm: force snapshot path
	pool.Register("fn")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inst, _ := pool.AcquireWithSnapshot("fn", store)
		pool.Release("fn", inst)
	}
}
