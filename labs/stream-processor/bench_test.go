// Benchmarks for the stream-processor lab.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Typical estimated results on an M2 MacBook Pro:
//
//	BenchmarkTumblingWindowProcess-10    8,500,000 ops/sec   ~118 ns/op
//	BenchmarkSlidingWindowProcess-10     2,800,000 ops/sec   ~357 ns/op
//	BenchmarkWindowFlush1000Keys-10        450,000 ops/sec  ~2222 ns/op
//	BenchmarkTxnCoordinator2PC-10          180,000 ops/sec  ~5556 ns/op
package bench_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/stream-processor/pkg/stream"
)

// BenchmarkTumblingWindowProcess measures per-event throughput for tumbling window accumulation.
// Target: >=5,000,000 events/sec (map write, no sorting).
func BenchmarkTumblingWindowProcess(b *testing.B) {
	tw := stream.NewTumblingWindow(time.Minute)
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		tw.Process(stream.Event{
			Key:       fmt.Sprintf("key-%d", i%100),
			Value:     float64(i),
			Timestamp: base, // same window — avoids boundary flushes
		})
	}
}

// BenchmarkSlidingWindowProcess measures per-event throughput for sliding window accumulation.
// Sorted buffer insertion is O(log n), making this slower than tumbling.
func BenchmarkSlidingWindowProcess(b *testing.B) {
	cfg := stream.ProcessorConfig{
		WindowSize:      5 * time.Minute,
		WindowStep:      1 * time.Minute,
		AllowedLateness: 5 * time.Second,
	}
	sp := stream.NewStreamProcessor(cfg)
	sp.Start()
	defer sp.Stop()

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sp.Source <- stream.Event{
			Key:       fmt.Sprintf("key-%d", i%100),
			Value:     float64(i),
			Timestamp: base.Add(time.Duration(i%300) * time.Second),
		}
	}
}

// BenchmarkWindowFlush1000Keys measures flush throughput when the window has 1000 distinct keys.
// This exercises map iteration + WindowResult allocation + channel write.
func BenchmarkWindowFlush1000Keys(b *testing.B) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tw := stream.NewTumblingWindow(time.Minute)
		for k := 0; k < 1000; k++ {
			tw.Process(stream.Event{
				Key:       fmt.Sprintf("sensor-%04d", k),
				Value:     float64(k),
				Timestamp: base,
			})
		}
		b.StartTimer()
		_ = tw.Flush()
	}
}

// BenchmarkTxnCoordinator2PC measures 2PC commit throughput with in-memory checkpoint store.
// The bottleneck is the atomic rename for the checkpoint file.
func BenchmarkTxnCoordinator2PC(b *testing.B) {
	dir, err := os.MkdirTemp("", "stream-bench-2pc-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	cpPath := dir + "/checkpoint.json"
	coord := stream.NewTxnCoordinator(cpPath)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []stream.Event{
		{Key: "k", Value: 1, Timestamp: base},
		{Key: "k", Value: 2, Timestamp: base.Add(time.Second)},
	}

	noop := func([]stream.WindowResult) error { return nil }

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := coord.ProcessBatch(events, noop); err != nil {
			b.Fatal(err)
		}
	}
}
