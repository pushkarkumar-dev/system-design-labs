package bench_test

import (
	"fmt"
	"testing"

	o "github.com/pushkar1005/system-design-labs/labs/container-orchestrator/pkg/orchestrator"
)

// BenchmarkSchedule measures the throughput of Scheduler.Schedule (first-fit)
// with 100 nodes each having ample capacity.
func BenchmarkSchedule(b *testing.B) {
	nodes := make([]*o.Node, 100)
	for i := range nodes {
		nodes[i] = &o.Node{
			Name:        fmt.Sprintf("node-%d", i),
			Capacity:    o.Resources{CPU: 32000, MemoryMB: 65536},
			Allocatable: o.Resources{CPU: 32000, MemoryMB: 65536},
		}
	}
	scheduler := o.NewScheduler(nodes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pod := &o.Pod{
			Name:    fmt.Sprintf("bench-pod-%d", i),
			Image:   "bench:latest",
			Request: o.Resources{CPU: 10, MemoryMB: 8},
		}
		if _, err := scheduler.Schedule(pod); err != nil {
			b.Fatalf("Schedule: %v", err)
		}
		// Restore allocatable so the benchmark is repeatable.
		nodes[0].Allocatable.CPU += pod.Request.CPU
		nodes[0].Allocatable.MemoryMB += pod.Request.MemoryMB
	}
}

// BenchmarkReconcileNoOp measures the reconcile hot path when desired state
// already matches actual state (no-op: map diff with 10 pods).
func BenchmarkReconcileNoOp(b *testing.B) {
	node := &o.Node{
		Name:        "bench-node",
		Capacity:    o.Resources{CPU: 32000, MemoryMB: 65536},
		Allocatable: o.Resources{CPU: 32000, MemoryMB: 65536},
	}
	reconciler := o.NewReconciler(o.NewScheduler([]*o.Node{node}))

	for i := 0; i < 10; i++ {
		pod := &o.Pod{
			Name:    fmt.Sprintf("bench-pod-%d", i),
			Image:   "app:v1",
			Request: o.Resources{CPU: 100, MemoryMB: 64},
		}
		reconciler.SetDesired(pod.Name, pod)
	}
	// Initial reconcile to populate actual state.
	if err := reconciler.Reconcile(); err != nil {
		b.Fatalf("setup Reconcile: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := reconciler.Reconcile(); err != nil {
			b.Fatalf("Reconcile: %v", err)
		}
	}
}

// BenchmarkStoreWatch measures the event delivery throughput of Store.Watch.
func BenchmarkStoreWatch(b *testing.B) {
	store := o.NewStore[*o.Pod]()
	ch := store.Watch()

	// Drain events in a background goroutine.
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pod := &o.Pod{Name: fmt.Sprintf("p-%d", i), Image: "app:v1"}
		_ = store.Add(fmt.Sprintf("p-%d", i), pod)
	}
	b.StopTimer()
}

// BenchmarkWorkQueueThroughput measures the enqueue + dequeue throughput of
// the WorkQueue with unique keys (no deduplication overhead).
func BenchmarkWorkQueueThroughput(b *testing.B) {
	wq := o.NewWorkQueue(0)

	// Pre-fill with unique keys.
	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = fmt.Sprintf("deployments/app-%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wq.Enqueue(keys[i])
		wq.Dequeue()
	}
}
