package orchestrator

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ── v0: Scheduler + Reconciler tests ──────────────────────────────────────────

// TestScheduleFitsOnNode verifies that a pod is scheduled to the first node
// that has sufficient allocatable resources.
func TestScheduleFitsOnNode(t *testing.T) {
	node := &Node{
		Name:        "node-1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	scheduler := NewScheduler([]*Node{node})

	pod := &Pod{
		Name:    "web-0",
		Image:   "nginx:latest",
		Request: Resources{CPU: 500, MemoryMB: 256},
	}

	selected, err := scheduler.Schedule(pod)
	if err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}
	if selected.Name != "node-1" {
		t.Errorf("selected node = %q, want 'node-1'", selected.Name)
	}
	if pod.Status != PodRunning {
		t.Errorf("pod status = %v, want Running", pod.Status)
	}
}

// TestScheduleFailsWhenNoResources verifies that Schedule returns an error
// when no node has sufficient allocatable resources.
func TestScheduleFailsWhenNoResources(t *testing.T) {
	node := &Node{
		Name:        "node-tiny",
		Capacity:    Resources{CPU: 100, MemoryMB: 128},
		Allocatable: Resources{CPU: 100, MemoryMB: 128},
	}
	scheduler := NewScheduler([]*Node{node})

	pod := &Pod{
		Name:    "bigpod",
		Image:   "heavy:latest",
		Request: Resources{CPU: 4000, MemoryMB: 16384},
	}

	_, err := scheduler.Schedule(pod)
	if err == nil {
		t.Fatal("expected error when no node has capacity, got nil")
	}
}

// TestReconcileCreatesMissingPod verifies that Reconcile schedules pods that
// are desired but not yet running.
func TestReconcileCreatesMissingPod(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	reconciler := NewReconciler(NewScheduler([]*Node{node}))

	desired := &Pod{
		Name:    "app-0",
		Image:   "myapp:v1",
		Request: Resources{CPU: 200, MemoryMB: 128},
	}
	reconciler.SetDesired("app-0", desired)

	if err := reconciler.Reconcile(); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := reconciler.ActualPod("app-0"); got == nil {
		t.Error("pod 'app-0' not found in actual state after Reconcile")
	}
}

// TestReconcileTerminatesExcess verifies that Reconcile removes pods that are
// no longer in the desired state.
func TestReconcileTerminatesExcess(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	reconciler := NewReconciler(NewScheduler([]*Node{node}))

	pod := &Pod{Name: "stale-pod", Image: "old:v1", Request: Resources{CPU: 100, MemoryMB: 64}}
	reconciler.SetDesired("stale-pod", pod)
	if err := reconciler.Reconcile(); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}

	// Remove from desired.
	reconciler.SetDesired("stale-pod", nil)
	if err := reconciler.Reconcile(); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}

	if reconciler.ActualPod("stale-pod") != nil {
		t.Error("pod 'stale-pod' still present after removal from desired state")
	}
}

// TestReconcileIsIdempotent verifies that running Reconcile twice when desired
// matches actual produces no changes and no errors.
func TestReconcileIsIdempotent(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	reconciler := NewReconciler(NewScheduler([]*Node{node}))

	for i := 0; i < 10; i++ {
		pod := &Pod{
			Name:    fmt.Sprintf("pod-%d", i),
			Image:   "app:v1",
			Request: Resources{CPU: 100, MemoryMB: 64},
		}
		reconciler.SetDesired(pod.Name, pod)
	}

	if err := reconciler.Reconcile(); err != nil {
		t.Fatalf("first Reconcile: %v", err)
	}
	countAfterFirst := reconciler.ActualCount()

	// Second reconcile should be a no-op.
	if err := reconciler.Reconcile(); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	if reconciler.ActualCount() != countAfterFirst {
		t.Errorf("actual count changed after idempotent reconcile: %d -> %d",
			countAfterFirst, reconciler.ActualCount())
	}
}

// TestResourceAccountingUpdatesOnSchedule verifies that scheduling a pod
// decrements the node's allocatable resources by the pod's request.
func TestResourceAccountingUpdatesOnSchedule(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	scheduler := NewScheduler([]*Node{node})

	pod := &Pod{
		Name:    "worker-0",
		Image:   "worker:latest",
		Request: Resources{CPU: 1000, MemoryMB: 512},
	}
	if _, err := scheduler.Schedule(pod); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if node.Allocatable.CPU != 3000 {
		t.Errorf("allocatable CPU = %d, want 3000", node.Allocatable.CPU)
	}
	if node.Allocatable.MemoryMB != 7680 {
		t.Errorf("allocatable MemoryMB = %d, want 7680", node.Allocatable.MemoryMB)
	}
}

// TestConcurrentReconcileSafe verifies that concurrent Reconcile calls do not
// race or corrupt state. Run with -race.
func TestConcurrentReconcileSafe(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 40000, MemoryMB: 81920},
		Allocatable: Resources{CPU: 40000, MemoryMB: 81920},
	}
	reconciler := NewReconciler(NewScheduler([]*Node{node}))

	for i := 0; i < 5; i++ {
		pod := &Pod{
			Name:    fmt.Sprintf("concurrent-pod-%d", i),
			Image:   "app:v1",
			Request: Resources{CPU: 100, MemoryMB: 64},
		}
		reconciler.SetDesired(pod.Name, pod)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = reconciler.Reconcile()
		}()
	}
	wg.Wait()
}

// TestNodeCapacityExhausted verifies that scheduling fails once a node's
// resources are fully allocated.
func TestNodeCapacityExhausted(t *testing.T) {
	node := &Node{
		Name:        "n-small",
		Capacity:    Resources{CPU: 1000, MemoryMB: 512},
		Allocatable: Resources{CPU: 1000, MemoryMB: 512},
	}
	scheduler := NewScheduler([]*Node{node})

	pod1 := &Pod{Name: "p1", Image: "app:v1", Request: Resources{CPU: 600, MemoryMB: 300}}
	if _, err := scheduler.Schedule(pod1); err != nil {
		t.Fatalf("first Schedule: %v", err)
	}

	pod2 := &Pod{Name: "p2", Image: "app:v1", Request: Resources{CPU: 600, MemoryMB: 300}}
	if _, err := scheduler.Schedule(pod2); err == nil {
		t.Fatal("expected error when node capacity is exhausted, got nil")
	}
}

// ── v1: Deployment + HealthChecker tests ─────────────────────────────────────

// TestDeploymentCreatesPods verifies that applying a Deployment schedules the
// correct number of pods.
func TestDeploymentCreatesPods(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 8000, MemoryMB: 16384},
		Allocatable: Resources{CPU: 8000, MemoryMB: 16384},
	}
	dc := NewDeploymentController(NewScheduler([]*Node{node}))

	d := &Deployment{
		Name:     "web",
		Image:    "nginx:1.24",
		Replicas: 3,
		Request:  Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	rs := dc.ReplicaSet("web")
	if rs == nil {
		t.Fatal("ReplicaSet for 'web' not found")
	}
	if len(rs.Pods) != 3 {
		t.Errorf("ReplicaSet has %d pods, want 3", len(rs.Pods))
	}
}

// TestRollingUpdateReplacesPods verifies that changing the Deployment image
// triggers a rolling update that replaces all old pods with new pods.
func TestRollingUpdateReplacesPods(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 8000, MemoryMB: 16384},
		Allocatable: Resources{CPU: 8000, MemoryMB: 16384},
	}
	dc := NewDeploymentController(NewScheduler([]*Node{node}))

	d1 := &Deployment{
		Name:           "api",
		Image:          "api:v1",
		Replicas:       2,
		UpdateStrategy: RollingUpdate{MaxSurge: 1, MaxUnavailable: 1},
		Request:        Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d1); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	d2 := &Deployment{
		Name:           "api",
		Image:          "api:v2",
		Replicas:       2,
		UpdateStrategy: RollingUpdate{MaxSurge: 1, MaxUnavailable: 1},
		Request:        Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d2); err != nil {
		t.Fatalf("Apply v2: %v", err)
	}

	rs := dc.ReplicaSet("api")
	if rs == nil {
		t.Fatal("ReplicaSet for 'api' not found after update")
	}
	if rs.Image != "api:v2" {
		t.Errorf("ReplicaSet image = %q, want 'api:v2'", rs.Image)
	}
	for _, pod := range rs.Pods {
		if pod.Image != "api:v2" {
			t.Errorf("pod %q still has old image %q", pod.Name, pod.Image)
		}
	}
}

// TestHealthCheckMarksFailed verifies that 3 consecutive health check failures
// mark the pod as Failed.
func TestHealthCheckMarksFailed(t *testing.T) {
	// Start a test HTTP server that always returns 500.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	dc := NewDeploymentController(NewScheduler([]*Node{node}))
	hc := NewHealthChecker(10*time.Millisecond, 3, dc)

	pod := &Pod{
		Name:           "sick-pod",
		Image:          "app:v1",
		Request:        Resources{CPU: 100, MemoryMB: 64},
		Status:         PodRunning,
		HealthEndpoint: srv.URL,
	}
	dc.pods[pod.Name] = pod
	hc.Register(pod)

	// Manually trigger 3 checks.
	for i := 0; i < 3; i++ {
		hc.checkAll()
	}

	if pod.Status != PodFailed {
		t.Errorf("pod status = %v after 3 failures, want Failed", pod.Status)
	}
}

// TestFailedPodTriggersReplacement verifies that after a pod is marked Failed,
// ReplaceFailedPods schedules a replacement pod.
func TestFailedPodTriggersReplacement(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 4000, MemoryMB: 8192},
		Allocatable: Resources{CPU: 4000, MemoryMB: 8192},
	}
	dc := NewDeploymentController(NewScheduler([]*Node{node}))

	pod := &Pod{
		Name:    "failing-pod",
		Image:   "app:v1",
		Request: Resources{CPU: 100, MemoryMB: 64},
		Status:  PodFailed,
	}
	dc.pods[pod.Name] = pod
	// Pre-allocate resources for the failed pod so the replacement can be scheduled.
	node.Allocatable.CPU -= pod.Request.CPU
	node.Allocatable.MemoryMB -= pod.Request.MemoryMB
	node.Pods = append(node.Pods, pod)

	if err := dc.ReplaceFailedPods(); err != nil {
		t.Fatalf("ReplaceFailedPods: %v", err)
	}

	// Original failed pod should be gone, replacement should exist.
	if _, stillThere := dc.pods["failing-pod"]; stillThere {
		t.Error("failed pod still present after replacement")
	}
	if len(dc.pods) == 0 {
		t.Error("no replacement pod was scheduled")
	}
}

// TestMaxSurgeLimitHonored verifies that the rolling update does not exceed
// MaxSurge extra pods above the desired replica count.
func TestMaxSurgeLimitHonored(t *testing.T) {
	node := &Node{
		Name:        "n1",
		Capacity:    Resources{CPU: 8000, MemoryMB: 16384},
		Allocatable: Resources{CPU: 8000, MemoryMB: 16384},
	}
	dc := NewDeploymentController(NewScheduler([]*Node{node}))

	d1 := &Deployment{
		Name:           "svc",
		Image:          "svc:v1",
		Replicas:       3,
		UpdateStrategy: RollingUpdate{MaxSurge: 1, MaxUnavailable: 1},
		Request:        Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d1); err != nil {
		t.Fatalf("Apply v1: %v", err)
	}

	d2 := &Deployment{
		Name:           "svc",
		Image:          "svc:v2",
		Replicas:       3,
		UpdateStrategy: RollingUpdate{MaxSurge: 1, MaxUnavailable: 1},
		Request:        Resources{CPU: 200, MemoryMB: 128},
	}
	if err := dc.Apply(d2); err != nil {
		t.Fatalf("Apply v2: %v", err)
	}

	rs := dc.ReplicaSet("svc")
	if len(rs.Pods) > d2.Replicas+d2.UpdateStrategy.MaxSurge {
		t.Errorf("ReplicaSet has %d pods, expected at most %d (replicas + maxSurge)",
			len(rs.Pods), d2.Replicas+d2.UpdateStrategy.MaxSurge)
	}
}

// ── v2: Store + Informer + WorkQueue tests ────────────────────────────────────

// TestStoreWatchDeliversEvents verifies that Add, Update, and Delete operations
// on a Store produce the correct WatchEvent[T] on the watch channel.
func TestStoreWatchDeliversEvents(t *testing.T) {
	store := NewStore[*Pod]()
	ch := store.Watch()

	pod := &Pod{Name: "test-pod", Image: "app:v1"}
	if err := store.Add("test-pod", pod); err != nil {
		t.Fatalf("Add: %v", err)
	}

	select {
	case event := <-ch:
		if event.Type != EventAdded {
			t.Errorf("event type = %v, want Added", event.Type)
		}
		if event.Object.Name != "test-pod" {
			t.Errorf("event object name = %q, want 'test-pod'", event.Object.Name)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for Added event")
	}

	pod.Image = "app:v2"
	if err := store.Update("test-pod", pod); err != nil {
		t.Fatalf("Update: %v", err)
	}
	select {
	case event := <-ch:
		if event.Type != EventModified {
			t.Errorf("event type = %v, want Modified", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for Modified event")
	}

	if err := store.Delete("test-pod"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	select {
	case event := <-ch:
		if event.Type != EventDeleted {
			t.Errorf("event type = %v, want Deleted", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for Deleted event")
	}
}

// TestInformerCallsHandler verifies that an Informer dispatches events to
// the registered EventHandler callbacks.
func TestInformerCallsHandler(t *testing.T) {
	store := NewStore[*Deployment]()
	inf := NewInformer(store)

	var mu sync.Mutex
	var added, updated, deleted int

	inf.AddEventHandler(EventHandler[*Deployment]{
		OnAdd: func(obj *Deployment) {
			mu.Lock()
			added++
			mu.Unlock()
		},
		OnUpdate: func(obj *Deployment) {
			mu.Lock()
			updated++
			mu.Unlock()
		},
		OnDelete: func(obj *Deployment) {
			mu.Lock()
			deleted++
			mu.Unlock()
		},
	})

	go inf.Run()
	defer inf.Stop()

	d := &Deployment{Name: "my-app", Image: "app:v1", Replicas: 2}
	_ = store.Add("my-app", d)
	d.Replicas = 3
	_ = store.Update("my-app", d)
	_ = store.Delete("my-app")

	// Allow event dispatch goroutine to process events.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if added != 1 {
		t.Errorf("OnAdd called %d times, want 1", added)
	}
	if updated != 1 {
		t.Errorf("OnUpdate called %d times, want 1", updated)
	}
	if deleted != 1 {
		t.Errorf("OnDelete called %d times, want 1", deleted)
	}
}

// TestSharedFactoryReturnsSameInformer verifies that calling
// SharedInformerFactory.DeploymentInformer with the same key twice returns
// the identical Informer instance.
func TestSharedFactoryReturnsSameInformer(t *testing.T) {
	factory := NewSharedInformerFactory()
	store := NewStore[*Deployment]()

	inf1 := factory.DeploymentInformer("deployments", store)
	inf2 := factory.DeploymentInformer("deployments", store)

	if inf1 != inf2 {
		t.Error("SharedInformerFactory returned different Informer instances for the same key")
	}
}

// TestWorkQueueDeduplicates verifies that enqueueing the same key 100 times
// results in only one item in the queue.
func TestWorkQueueDeduplicates(t *testing.T) {
	wq := NewWorkQueue(0) // unbounded

	const key = "deployments/my-app"
	for i := 0; i < 100; i++ {
		wq.Enqueue(key)
	}

	if wq.Len() != 1 {
		t.Errorf("queue length = %d after 100 identical enqueues, want 1", wq.Len())
	}

	got, ok := wq.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned false on non-empty queue")
	}
	if got != key {
		t.Errorf("dequeued key = %q, want %q", got, key)
	}
}

// TestControllerReactsToInformerEvent verifies the full v2 wiring: a Deployment
// change in the Store is delivered via an Informer to a controller that enqueues
// the key in a WorkQueue.
func TestControllerReactsToInformerEvent(t *testing.T) {
	store := NewStore[*Deployment]()
	inf := NewInformer(store)
	wq := NewWorkQueue(0)

	inf.AddEventHandler(EventHandler[*Deployment]{
		OnAdd: func(obj *Deployment) {
			wq.Enqueue("deployments/" + obj.Name)
		},
		OnUpdate: func(obj *Deployment) {
			wq.Enqueue("deployments/" + obj.Name)
		},
	})

	go inf.Run()
	defer inf.Stop()

	d := &Deployment{Name: "reactive-app", Image: "app:v1", Replicas: 1}
	_ = store.Add("reactive-app", d)

	time.Sleep(50 * time.Millisecond)

	if wq.Len() != 1 {
		t.Errorf("work queue length = %d after informer event, want 1", wq.Len())
	}

	key, ok := wq.Dequeue()
	if !ok {
		t.Fatal("Dequeue returned false")
	}
	if key != "deployments/reactive-app" {
		t.Errorf("dequeued key = %q, want 'deployments/reactive-app'", key)
	}
}
