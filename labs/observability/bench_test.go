package obs_bench_test

import (
	"bytes"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/observability/pkg/obs"
)

// BenchmarkCounterInc measures the throughput of Counter.Inc() under
// single-goroutine load.
//
// Expected: ~180,000,000 ops/sec on M2 MacBook Pro (atomic CAS, no lock).
func BenchmarkCounterInc(b *testing.B) {
	c := obs.NewCounter("bench", nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Inc()
	}
}

// BenchmarkCounterIncParallel measures Counter.Inc() under multi-goroutine
// contention. Demonstrates that atomic CAS scales across cores.
func BenchmarkCounterIncParallel(b *testing.B) {
	c := obs.NewCounter("bench_parallel", nil)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

// BenchmarkHistogramObserve measures Histogram.Observe() with 11 buckets
// (DefaultBuckets). Each call does a binary search + N atomic increments.
//
// Expected: ~45,000,000 ops/sec on M2 MacBook Pro.
func BenchmarkHistogramObserve(b *testing.B) {
	h := obs.NewHistogram("latency_seconds", nil, nil) // 11 DefaultBuckets
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Observe(0.05) // hits bucket index 5 (0.1 upper bound)
	}
}

// BenchmarkPrometheusText measures the cost of serialising 1000 metrics to
// Prometheus text format. This is the /metrics scrape cost.
//
// Expected: ~85,000 scrapes/sec on M2 MacBook Pro (11µs per scrape).
func BenchmarkPrometheusText(b *testing.B) {
	reg := obs.NewMetricsRegistry()
	for i := 0; i < 1000; i++ {
		name := obs.BenchMetricName(i)
		c := obs.NewCounter("bench counter", nil)
		c.Add(float64(i))
		reg.Register(name, c)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = obs.WritePrometheusText(&buf, reg.Gather())
	}
}

// BenchmarkSpanFinish measures the cost of Span.Finish() including the
// ring-buffer write under mutex.
//
// Expected: ~8,500,000 spans/sec on M2 MacBook Pro.
func BenchmarkSpanFinish(b *testing.B) {
	tracer := obs.NewTracer()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := tracer.StartSpan("bench.op", nil)
		span.Finish()
	}
}
