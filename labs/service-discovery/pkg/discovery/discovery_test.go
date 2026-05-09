// discovery_test.go — tests for all three stages of the service discovery lab.
//
// v0 tests: Register, Deregister, Lookup, LookupByTag, concurrency.
// v1 tests: TTL expiry, heartbeat prevents expiry, Watch events.
// v2 tests: round-robin, empty-service error, cache refresh, ResolveAll.
package discovery

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// ── v0: Registry tests ────────────────────────────────────────────────────────

func makeInstance(id, svc, host, port string, tags ...string) ServiceInstance {
	return ServiceInstance{
		ID:          id,
		ServiceName: svc,
		Host:        host,
		Port:        port,
		Tags:        tags,
		Metadata:    map[string]string{},
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	inst := makeInstance("a1", "payment", "10.0.0.1", "8080")
	if err := r.Register(inst); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got := r.Lookup("payment")
	if len(got) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(got))
	}
	if got[0].ID != "a1" {
		t.Fatalf("expected ID a1, got %s", got[0].ID)
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	inst := makeInstance("a1", "payment", "10.0.0.1", "8080")
	_ = r.Register(inst)
	if err := r.Register(inst); err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
}

func TestRegistry_Deregister(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(makeInstance("a1", "payment", "10.0.0.1", "8080"))
	if err := r.Deregister("a1"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	got := r.Lookup("payment")
	if len(got) != 0 {
		t.Fatalf("expected 0 instances after deregister, got %d", len(got))
	}
}

func TestRegistry_DeregisterMissing(t *testing.T) {
	r := NewRegistry()
	if err := r.Deregister("nonexistent"); err == nil {
		t.Fatal("expected error for missing ID, got nil")
	}
}

func TestRegistry_Lookup_MultipleInstances(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 5; i++ {
		_ = r.Register(makeInstance(
			fmt.Sprintf("a%d", i), "payment",
			fmt.Sprintf("10.0.0.%d", i), "8080",
		))
	}
	got := r.Lookup("payment")
	if len(got) != 5 {
		t.Fatalf("expected 5 instances, got %d", len(got))
	}
}

func TestRegistry_LookupUnknownService(t *testing.T) {
	r := NewRegistry()
	got := r.Lookup("nonexistent")
	if got == nil {
		t.Fatal("expected non-nil slice for unknown service")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %d items", len(got))
	}
}

func TestRegistry_LookupByTag(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(makeInstance("a1", "payment", "10.0.0.1", "8080", "primary", "us-east"))
	_ = r.Register(makeInstance("a2", "payment", "10.0.0.2", "8080", "replica", "us-west"))
	_ = r.Register(makeInstance("a3", "payment", "10.0.0.3", "8080", "primary", "eu-west"))

	got := r.LookupByTag("payment", "primary")
	if len(got) != 2 {
		t.Fatalf("expected 2 primary instances, got %d", len(got))
	}
	for _, inst := range got {
		if !inst.HasTag("primary") {
			t.Fatalf("instance %s does not have 'primary' tag", inst.ID)
		}
	}
}

func TestRegistry_ConcurrentRegistration(t *testing.T) {
	r := NewRegistry()
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = r.Register(makeInstance(
				fmt.Sprintf("inst-%d", i), "load-test",
				"10.0.0.1", fmt.Sprintf("%d", 8000+i),
			))
		}(i)
	}
	wg.Wait()
	got := r.Lookup("load-test")
	if len(got) != n {
		t.Fatalf("expected %d instances after concurrent registration, got %d", n, len(got))
	}
}

// ── v1: HealthRegistry tests ──────────────────────────────────────────────────

func TestHealthRegistry_TTLExpiry(t *testing.T) {
	ttl := 200 * time.Millisecond
	hr := NewHealthRegistry(ttl)
	defer hr.Stop()

	inst := makeInstance("b1", "orders", "10.0.1.1", "9090")
	if err := hr.Register(inst, ttl); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Verify instance is visible before expiry.
	if got := hr.Lookup("orders"); len(got) != 1 {
		t.Fatalf("expected 1 instance before expiry, got %d", len(got))
	}

	// Wait for TTL + cleaner interval to pass.
	time.Sleep(ttl + ttl/3 + 50*time.Millisecond)

	got := hr.Lookup("orders")
	if len(got) != 0 {
		t.Fatalf("expected 0 instances after TTL expiry, got %d", len(got))
	}
}

func TestHealthRegistry_HeartbeatPreventsExpiry(t *testing.T) {
	ttl := 300 * time.Millisecond
	hr := NewHealthRegistry(ttl)
	defer hr.Stop()

	inst := makeInstance("b2", "inventory", "10.0.1.2", "9091")
	if err := hr.Register(inst, ttl); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Send heartbeats at intervals shorter than TTL to keep the instance alive.
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = hr.Heartbeat("b2")
			case <-done:
				return
			}
		}
	}()

	// Sleep for 2× TTL; heartbeats should have kept the instance alive.
	time.Sleep(2 * ttl)
	close(done)

	got := hr.Lookup("inventory")
	if len(got) != 1 {
		t.Fatalf("expected instance to survive with heartbeats, got %d instances", len(got))
	}
}

func TestHealthRegistry_WatchReceivesAddEvent(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	events := hr.Watch("shipping")

	inst := makeInstance("c1", "shipping", "10.0.2.1", "7070")
	if err := hr.Register(inst, 30*time.Second); err != nil {
		t.Fatalf("Register: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Type != EventAdded {
			t.Fatalf("expected EventAdded, got %s", ev.Type)
		}
		if ev.Instance.ID != "c1" {
			t.Fatalf("expected ID c1, got %s", ev.Instance.ID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for Watch add event")
	}
}

func TestHealthRegistry_WatchReceivesRemoveOnDeregister(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	inst := makeInstance("c2", "auth", "10.0.2.2", "7071")
	_ = hr.Register(inst, 30*time.Second)

	events := hr.Watch("auth")

	// Drain the buffer in case the register event landed before Watch was called.
	// (It won't in this test since Watch is called after Register, but be defensive.)

	if err := hr.Deregister("c2"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}

	select {
	case ev := <-events:
		if ev.Type != EventRemoved {
			t.Fatalf("expected EventRemoved, got %s", ev.Type)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for Watch remove event")
	}
}

func TestHealthRegistry_WatchReceivesExpiry(t *testing.T) {
	ttl := 150 * time.Millisecond
	hr := NewHealthRegistry(ttl)
	defer hr.Stop()

	inst := makeInstance("c3", "cache", "10.0.2.3", "6379")
	_ = hr.Register(inst, ttl)

	events := hr.Watch("cache")

	// Wait for the expiry cleaner to fire.
	select {
	case ev := <-events:
		if ev.Type != EventRemoved {
			t.Fatalf("expected EventRemoved on TTL expiry, got %s", ev.Type)
		}
		if ev.Instance.ID != "c3" {
			t.Fatalf("expected ID c3, got %s", ev.Instance.ID)
		}
	case <-time.After(ttl*3 + 200*time.Millisecond):
		t.Fatal("timed out waiting for TTL expiry event")
	}
}

func TestHealthRegistry_MultipleWatchersSameService(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	// Two independent watchers on "billing".
	ch1 := hr.Watch("billing")
	ch2 := hr.Watch("billing")

	inst := makeInstance("d1", "billing", "10.0.3.1", "5000")
	_ = hr.Register(inst, 30*time.Second)

	got1, got2 := false, false
	timeout := time.After(500 * time.Millisecond)

	for !got1 || !got2 {
		select {
		case ev := <-ch1:
			if ev.Type == EventAdded {
				got1 = true
			}
		case ev := <-ch2:
			if ev.Type == EventAdded {
				got2 = true
			}
		case <-timeout:
			t.Fatalf("timed out: watcher1=%v watcher2=%v", got1, got2)
		}
	}
}

// ── v2: ServiceClient tests ───────────────────────────────────────────────────

func TestServiceClient_RoundRobin(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	// Register 3 instances.
	for i := 0; i < 3; i++ {
		_ = hr.Register(makeInstance(
			fmt.Sprintf("e%d", i), "frontend",
			fmt.Sprintf("10.0.4.%d", i), "3000",
		), 30*time.Second)
	}

	sc := NewServiceClient(hr)
	defer sc.Stop()

	// Collect resolved hosts over 9 calls (3 rounds).
	seen := make(map[string]int)
	for i := 0; i < 9; i++ {
		host, _, err := sc.Resolve("frontend")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		seen[host]++
	}

	// Each of the 3 hosts should be selected exactly 3 times.
	if len(seen) != 3 {
		t.Fatalf("expected 3 distinct hosts in round-robin, got %d: %v", len(seen), seen)
	}
	for host, count := range seen {
		if count != 3 {
			t.Errorf("host %s selected %d times, expected 3", host, count)
		}
	}
}

func TestServiceClient_EmptyServiceNameError(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()
	sc := NewServiceClient(hr)
	defer sc.Stop()

	_, _, err := sc.Resolve("")
	if err == nil {
		t.Fatal("expected error for empty service name, got nil")
	}
}

func TestServiceClient_GracefulDegradationWhenAllExpired(t *testing.T) {
	ttl := 100 * time.Millisecond
	hr := NewHealthRegistry(ttl)
	defer hr.Stop()

	_ = hr.Register(makeInstance("f1", "analytics", "10.0.5.1", "9200"), ttl)

	sc := NewServiceClient(hr)
	defer sc.Stop()

	// Verify it resolves while alive.
	if _, _, err := sc.Resolve("analytics"); err != nil {
		t.Fatalf("initial Resolve failed: %v", err)
	}

	// Wait for TTL expiry.
	time.Sleep(ttl + ttl/3 + 100*time.Millisecond)

	_, _, err := sc.Resolve("analytics")
	if err != ErrNoInstances {
		t.Fatalf("expected ErrNoInstances after expiry, got %v", err)
	}
}

func TestServiceClient_CacheRefreshOnWatchEvent(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	sc := NewServiceClient(hr)
	defer sc.Stop()

	// Initially no instances.
	_, _, err := sc.Resolve("search")
	if err != ErrNoInstances {
		t.Fatalf("expected ErrNoInstances before any registration, got %v", err)
	}

	// Register a new instance. The Watch goroutine should update the cache.
	_ = hr.Register(makeInstance("g1", "search", "10.0.6.1", "9300"), 30*time.Second)

	// Give the Watch goroutine time to process the event.
	time.Sleep(50 * time.Millisecond)

	host, port, err := sc.Resolve("search")
	if err != nil {
		t.Fatalf("expected Resolve to succeed after registration, got %v", err)
	}
	if host != "10.0.6.1" || port != "9300" {
		t.Fatalf("unexpected resolved endpoint: %s:%s", host, port)
	}
}

func TestServiceClient_ResolveAll(t *testing.T) {
	hr := NewHealthRegistry(30 * time.Second)
	defer hr.Stop()

	for i := 0; i < 4; i++ {
		_ = hr.Register(makeInstance(
			fmt.Sprintf("h%d", i), "gateway",
			fmt.Sprintf("10.0.7.%d", i), "8080",
		), 30*time.Second)
	}

	sc := NewServiceClient(hr)
	defer sc.Stop()

	// Seed the cache.
	_, _, _ = sc.Resolve("gateway")

	endpoints := sc.ResolveAll("gateway")
	if len(endpoints) != 4 {
		t.Fatalf("expected 4 endpoints from ResolveAll, got %d", len(endpoints))
	}
	for _, ep := range endpoints {
		if ep.ServiceName != "gateway" {
			t.Errorf("unexpected service name %q in endpoint", ep.ServiceName)
		}
		if ep.Weight != 1 {
			t.Errorf("expected weight 1, got %d", ep.Weight)
		}
	}
}
