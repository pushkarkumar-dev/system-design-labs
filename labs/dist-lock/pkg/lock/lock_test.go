package lock_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/dist-lock/pkg/lock"
)

// ---------------------------------------------------------------------------
// v0: LockManager (lease-based) tests
// ---------------------------------------------------------------------------

// TestAcquire_SucceedsWhenFree verifies that acquiring an unlocked resource
// returns a positive token and ok=true.
func TestAcquire_SucceedsWhenFree(t *testing.T) {
	m := lock.NewLockManager()
	token, ok := m.Acquire("orders", "svc-a", 5*time.Second)
	if !ok {
		t.Fatal("expected acquire to succeed on free resource")
	}
	if token <= 0 {
		t.Fatalf("expected positive fencing token, got %d", token)
	}
}

// TestAcquire_FailsWhenHeld verifies that a second Acquire on a held resource
// returns ok=false while the first holder's TTL is still active.
func TestAcquire_FailsWhenHeld(t *testing.T) {
	m := lock.NewLockManager()
	_, ok := m.Acquire("payments", "svc-a", 5*time.Second)
	if !ok {
		t.Fatal("first acquire should succeed")
	}

	_, ok2 := m.Acquire("payments", "svc-b", 5*time.Second)
	if ok2 {
		t.Fatal("second acquire on held resource should fail")
	}
}

// TestAcquire_SucceedsAfterExpiry verifies the self-healing property of leases:
// once the TTL elapses, any caller can re-acquire the resource without an
// explicit Release from the crashed holder.
func TestAcquire_SucceedsAfterExpiry(t *testing.T) {
	m := lock.NewLockManager()
	token1, ok := m.Acquire("billing", "svc-a", 50*time.Millisecond)
	if !ok {
		t.Fatal("first acquire should succeed")
	}

	// Wait for TTL to elapse.
	time.Sleep(100 * time.Millisecond)

	token2, ok2 := m.Acquire("billing", "svc-b", 5*time.Second)
	if !ok2 {
		t.Fatal("re-acquire after TTL expiry should succeed")
	}
	if token2 <= token1 {
		t.Errorf("second token must be greater than first: token1=%d token2=%d", token1, token2)
	}
}

// TestRelease_RequiresMatchingToken verifies that Release only succeeds when
// the caller passes the token it received from Acquire. A stale or wrong token
// must not release the current holder's lock.
func TestRelease_RequiresMatchingToken(t *testing.T) {
	m := lock.NewLockManager()
	token, ok := m.Acquire("inventory", "svc-a", 5*time.Second)
	if !ok {
		t.Fatal("acquire should succeed")
	}

	// Wrong token: should not release.
	if m.Release("inventory", token+999) {
		t.Fatal("release with wrong token should return false")
	}

	// Correct token: should release.
	if !m.Release("inventory", token) {
		t.Fatal("release with correct token should return true")
	}

	// Verify resource is now free.
	if m.State("inventory") != nil {
		t.Fatal("resource should be free after release")
	}
}

// TestRelease_DoesNotReleaseNewHolder verifies the critical safety property:
// if the original holder's TTL expires and a new holder acquires the resource,
// the original holder's stale Release must not evict the new holder.
func TestRelease_DoesNotReleaseNewHolder(t *testing.T) {
	m := lock.NewLockManager()

	// svc-a acquires with short TTL.
	token1, ok := m.Acquire("catalog", "svc-a", 30*time.Millisecond)
	if !ok {
		t.Fatal("first acquire should succeed")
	}

	// TTL expires.
	time.Sleep(60 * time.Millisecond)

	// svc-b acquires the now-free resource.
	token2, ok2 := m.Acquire("catalog", "svc-b", 5*time.Second)
	if !ok2 {
		t.Fatal("svc-b should acquire after expiry")
	}

	// svc-a wakes up (e.g., from GC pause) and tries to release with its old token.
	released := m.Release("catalog", token1)
	if released {
		t.Fatal("stale Release by svc-a must not release svc-b's lock")
	}

	// svc-b should still hold the lock.
	state := m.State("catalog")
	if state == nil {
		t.Fatal("svc-b's lock should still be active")
	}
	if state.Token != token2 {
		t.Errorf("expected token2=%d, got %d", token2, state.Token)
	}
}

// TestRenew_ExtendsActiveLock verifies that Renew refreshes the TTL for the
// correct owner+token and returns false for mismatched or expired locks.
func TestRenew_ExtendsActiveLock(t *testing.T) {
	m := lock.NewLockManager()
	token, ok := m.Acquire("search", "svc-a", 200*time.Millisecond)
	if !ok {
		t.Fatal("acquire should succeed")
	}

	time.Sleep(100 * time.Millisecond)

	// Renew should succeed and extend TTL.
	if !m.Renew("search", "svc-a", token, 5*time.Second) {
		t.Fatal("renew should succeed for correct owner+token")
	}

	// Wait past original TTL — lock should still be held due to renewal.
	time.Sleep(200 * time.Millisecond)
	if m.State("search") == nil {
		t.Fatal("lock should still be active after renewal")
	}

	// Wrong owner should not be able to renew.
	if m.Renew("search", "svc-b", token, 5*time.Second) {
		t.Fatal("renew with wrong owner should fail")
	}
}

// ---------------------------------------------------------------------------
// v1: StorageServer fencing token tests
// ---------------------------------------------------------------------------

// TestFencingToken_RejectsStaleWrite verifies the core guarantee of fencing tokens:
// a write with a token lower than the previously accepted token is rejected.
// This simulates a process that woke up from a GC pause after its lock expired.
func TestFencingToken_RejectsStaleWrite(t *testing.T) {
	m := lock.NewLockManager()
	storage := lock.NewStorageServer()

	// Process A acquires lock with token=1 and writes.
	tokenA, ok := m.Acquire("user-profile", "process-a", 50*time.Millisecond)
	if !ok {
		t.Fatal("process-a acquire should succeed")
	}
	if err := storage.Write("profile:42", "data-from-A", tokenA); err != nil {
		t.Fatalf("process-a first write should succeed: %v", err)
	}

	// Process A is GC paused. Lock expires. Process B acquires with token=2.
	time.Sleep(100 * time.Millisecond)
	tokenB, ok2 := m.Acquire("user-profile", "process-b", 5*time.Second)
	if !ok2 {
		t.Fatal("process-b acquire after expiry should succeed")
	}
	if tokenB <= tokenA {
		t.Fatalf("tokenB must be > tokenA: tokenA=%d tokenB=%d", tokenA, tokenB)
	}

	// Process B writes with its newer token.
	if err := storage.Write("profile:42", "data-from-B", tokenB); err != nil {
		t.Fatalf("process-b write should succeed: %v", err)
	}

	// Process A wakes up from GC pause and tries to write with its stale token.
	err := storage.Write("profile:42", "stale-data-from-A", tokenA)
	if err == nil {
		t.Fatal("stale write from process-a should be rejected by storage server")
	}

	// Verify storage still has process-b's data.
	val, _, found := storage.Read("profile:42")
	if !found || val != "data-from-B" {
		t.Errorf("storage should have process-b's data, got val=%q found=%v", val, found)
	}
}

// TestFencingToken_AcceptsHigherToken verifies that the storage server allows
// writes with equal or higher tokens.
func TestFencingToken_AcceptsHigherToken(t *testing.T) {
	storage := lock.NewStorageServer()

	if err := storage.Write("key", "v1", 5); err != nil {
		t.Fatalf("first write should succeed: %v", err)
	}
	if err := storage.Write("key", "v2", 5); err != nil {
		t.Fatalf("write with equal token should succeed: %v", err)
	}
	if err := storage.Write("key", "v3", 10); err != nil {
		t.Fatalf("write with higher token should succeed: %v", err)
	}

	err := storage.Write("key", "stale", 3)
	if err == nil {
		t.Fatal("write with lower token should be rejected")
	}
}

// ---------------------------------------------------------------------------
// Concurrency: multiple goroutines competing for one lock
// ---------------------------------------------------------------------------

// TestConcurrentAcquire_OnlyOneSucceeds verifies that under concurrent contention
// for the same resource, at most one goroutine holds the lock at any instant.
// This is the fundamental mutual exclusion property.
func TestConcurrentAcquire_OnlyOneSucceeds(t *testing.T) {
	m := lock.NewLockManager()
	const goroutines = 10
	const iterations = 100

	var (
		wg            sync.WaitGroup
		maxConcurrent int32
		current       int32
		violations    int32
	)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			owner := fmt.Sprintf("worker-%d", id)
			for i := 0; i < iterations; i++ {
				token, ok := m.Acquire("shared-resource", owner, 100*time.Millisecond)
				if !ok {
					continue
				}

				// Count concurrent holders — should never exceed 1.
				c := atomic.AddInt32(&current, 1)
				if c > 1 {
					atomic.AddInt32(&violations, 1)
				}
				if c > atomic.LoadInt32(&maxConcurrent) {
					atomic.StoreInt32(&maxConcurrent, c)
				}

				// Simulate brief critical section.
				time.Sleep(time.Microsecond)

				atomic.AddInt32(&current, -1)
				m.Release("shared-resource", token)
			}
		}(g)
	}

	wg.Wait()

	if violations > 0 {
		t.Errorf("mutual exclusion violated: %d concurrent holder instances detected", violations)
	}
	t.Logf("max concurrent holders observed: %d (should be 1)", maxConcurrent)
}
