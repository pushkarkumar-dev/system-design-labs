// Command server is the entry point for the container-orchestrator demo.
//
// It demonstrates the full v0→v2 stack:
//   - v0: Scheduler + Reconciler — schedules pods onto nodes and reconciles state
//   - v1: DeploymentController — rolling update from v1 to v2 image
//   - v2: Store + Informer + WorkQueue — event-driven controller loop
//
// Usage:
//
//	go run ./cmd/server/
//
// The server prints a structured walk-through and exits. There is no persistent
// daemon — this is a teaching tool, not a production orchestrator.
package main

import (
	"fmt"
	"time"

	o "github.com/pushkar1005/system-design-labs/labs/container-orchestrator/pkg/orchestrator"
)

func main() {
	fmt.Println("=== Container Orchestrator Demo ===")
	fmt.Println()

	// ── v0: Scheduler + Reconcile Loop ────────────────────────────────────────
	fmt.Println("── v0: Scheduler + Reconcile Loop ──")

	nodes := []*o.Node{
		{Name: "node-1", Capacity: o.Resources{CPU: 4000, MemoryMB: 8192}, Allocatable: o.Resources{CPU: 4000, MemoryMB: 8192}},
		{Name: "node-2", Capacity: o.Resources{CPU: 4000, MemoryMB: 8192}, Allocatable: o.Resources{CPU: 4000, MemoryMB: 8192}},
	}
	scheduler := o.NewScheduler(nodes)
	reconciler := o.NewReconciler(scheduler)

	for i := 0; i < 3; i++ {
		pod := &o.Pod{
			Name:    fmt.Sprintf("web-%d", i),
			Image:   "nginx:1.25",
			Request: o.Resources{CPU: 500, MemoryMB: 256},
		}
		reconciler.SetDesired(pod.Name, pod)
	}

	if err := reconciler.Reconcile(); err != nil {
		fmt.Printf("Reconcile error: %v\n", err)
	}
	fmt.Printf("After first reconcile: %d pods running\n", reconciler.ActualCount())

	// Remove one pod from desired state.
	reconciler.SetDesired("web-2", nil)
	if err := reconciler.Reconcile(); err != nil {
		fmt.Printf("Reconcile error: %v\n", err)
	}
	fmt.Printf("After removing web-2: %d pods running\n", reconciler.ActualCount())
	fmt.Println()

	// ── v1: Deployment Controller ─────────────────────────────────────────────
	fmt.Println("── v1: Deployment Controller ──")

	nodes2 := []*o.Node{
		{Name: "n1", Capacity: o.Resources{CPU: 8000, MemoryMB: 16384}, Allocatable: o.Resources{CPU: 8000, MemoryMB: 16384}},
	}
	dc := o.NewDeploymentController(o.NewScheduler(nodes2))

	d1 := &o.Deployment{
		Name:           "api",
		Image:          "api:v1",
		Replicas:       3,
		UpdateStrategy: o.RollingUpdate{MaxSurge: 1, MaxUnavailable: 1},
		Request:        o.Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d1); err != nil {
		fmt.Printf("Apply v1: %v\n", err)
	}
	rs1 := dc.ReplicaSet("api")
	fmt.Printf("Deployment api v1: %d pods, image=%s\n", len(rs1.Pods), rs1.Image)

	d2 := &o.Deployment{
		Name:           "api",
		Image:          "api:v2",
		Replicas:       3,
		UpdateStrategy: o.RollingUpdate{MaxSurge: 1, MaxUnavailable: 1},
		Request:        o.Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d2); err != nil {
		fmt.Printf("Apply v2: %v\n", err)
	}
	rs2 := dc.ReplicaSet("api")
	fmt.Printf("Deployment api v2: %d pods, image=%s\n", len(rs2.Pods), rs2.Image)
	fmt.Println()

	// ── v2: Store + Informer + WorkQueue ─────────────────────────────────────
	fmt.Println("── v2: Store + Informer + WorkQueue ──")

	store := o.NewStore[*o.Deployment]()
	wq := o.NewWorkQueue(100)
	inf := o.NewInformer(store)

	inf.AddEventHandler(o.EventHandler[*o.Deployment]{
		OnAdd: func(dep *o.Deployment) {
			wq.Enqueue("deployments/" + dep.Name)
			fmt.Printf("  [informer] Added deployment %q -> enqueued\n", dep.Name)
		},
		OnUpdate: func(dep *o.Deployment) {
			wq.Enqueue("deployments/" + dep.Name)
			fmt.Printf("  [informer] Updated deployment %q -> enqueued (deduped)\n", dep.Name)
		},
	})

	go inf.Run()

	_ = store.Add("frontend", &o.Deployment{Name: "frontend", Image: "fe:v1", Replicas: 2})
	fe := &o.Deployment{Name: "frontend", Image: "fe:v2", Replicas: 2}
	_ = store.Update("frontend", fe)
	_ = store.Update("frontend", fe) // duplicate — should be deduped in queue

	time.Sleep(50 * time.Millisecond)
	inf.Stop()

	fmt.Printf("  WorkQueue length: %d (should be 1 after dedup)\n", wq.Len())

	fmt.Println()
	fmt.Println("Demo complete.")
}
