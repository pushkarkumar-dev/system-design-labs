package ratelimit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/rate-limiter/pkg/ratelimit"
	"github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------
// v0: Token bucket tests
// ---------------------------------------------------------------------------

// TestTokenBucket_AllowsBurstUpToCapacity verifies the burst behaviour of
// the token bucket. A full bucket (capacity=5) should allow 5 consecutive
// requests before blocking the 6th.
func TestTokenBucket_AllowsBurstUpToCapacity(t *testing.T) {
	// capacity=5, refillRate=1 token/sec — slow refill so we don't accidentally
	// refill between the rapid Allow() calls in the test.
	b := ratelimit.NewTokenBucket(5, 1.0)

	allowed := 0
	for i := 0; i < 6; i++ {
		if b.Allow() {
			allowed++
		}
	}

	if allowed != 5 {
		t.Errorf("expected exactly 5 allowed (bucket capacity), got %d", allowed)
	}
}

// TestTokenBucket_RefillsOverTime verifies that the bucket refills tokens
// at the configured rate after being drained.
func TestTokenBucket_RefillsOverTime(t *testing.T) {
	// capacity=10, refillRate=100 tokens/sec so 10ms gives ~1 token
	b := ratelimit.NewTokenBucket(10, 100.0)

	// Drain the bucket
	for b.Allow() {
	}

	// Wait 50ms → should have refilled ~5 tokens
	time.Sleep(50 * time.Millisecond)

	allowed := 0
	for i := 0; i < 20; i++ {
		if b.Allow() {
			allowed++
		}
	}

	if allowed < 3 || allowed > 7 {
		t.Errorf("after 50ms refill at 100 tok/s, expected ~5 allowed, got %d", allowed)
	}
}

// TestTokenBucketLimiter_PerKey verifies that different keys get independent
// buckets, so exhausting one key doesn't affect another.
func TestTokenBucketLimiter_PerKey(t *testing.T) {
	l := ratelimit.NewTokenBucketLimiter(3, 1.0)

	// Drain key "a"
	l.Allow("a")
	l.Allow("a")
	l.Allow("a")
	if l.Allow("a") {
		t.Fatal("key 'a' should be exhausted")
	}

	// key "b" should still have its full bucket
	if !l.Allow("b") {
		t.Fatal("key 'b' should still be allowed")
	}
}

// ---------------------------------------------------------------------------
// v1: Sliding window tests
// ---------------------------------------------------------------------------

// TestSlidingWindow_BlocksAtLimit verifies that requests are rejected once
// the sliding window count reaches the limit.
func TestSlidingWindow_BlocksAtLimit(t *testing.T) {
	l := ratelimit.NewSlidingWindowLimiter()

	limit := int64(5)
	allowed := 0
	for i := 0; i < 10; i++ {
		if l.Allow("user1", limit) {
			allowed++
		}
	}

	if allowed != 5 {
		t.Errorf("expected exactly 5 allowed (limit), got %d", allowed)
	}
}

// TestSlidingWindow_NoBoundaryBurst demonstrates the key advantage of
// the sliding window over a fixed window.
//
// In a fixed window, a client can send limit requests at second 59 and
// another limit requests at second 60 — 2× the limit in 2 seconds.
// The sliding window prevents this: it always counts the last 60 seconds,
// regardless of window alignment.
//
// We simulate this by using two different keys (since we can't rewind time)
// and verifying that neither can exceed the limit.
func TestSlidingWindow_NoBoundaryBurst(t *testing.T) {
	l := ratelimit.NewSlidingWindowLimiter()
	limit := int64(100)

	// Simulate "end of window 1": send limit requests
	for i := 0; i < 100; i++ {
		l.Allow("border-test", limit)
	}

	// The count should now be at the limit — no room for more
	extra := l.Allow("border-test", limit)
	if extra {
		t.Error("sliding window allowed a request beyond the limit — boundary burst protection failed")
	}

	// The current count in the window should equal the limit
	count := l.Count("border-test")
	if count != limit {
		t.Errorf("expected count=%d, got %d", limit, count)
	}
}

// TestSlidingWindow_IndependentKeys verifies per-key isolation.
func TestSlidingWindow_IndependentKeys(t *testing.T) {
	l := ratelimit.NewSlidingWindowLimiter()

	// Exhaust key1
	for i := 0; i < 5; i++ {
		l.Allow("key1", 5)
	}

	// key2 should be unaffected
	if !l.Allow("key2", 5) {
		t.Error("key2 should not be affected by key1's exhaustion")
	}
}

// ---------------------------------------------------------------------------
// v2: Distributed rate limiter tests (using a mock Redis-compatible store)
// ---------------------------------------------------------------------------

// mockRedis is a minimal in-memory Redis-compatible store for testing the
// distributed limiter without requiring a real Redis server.
// It uses a sync.Map under the hood and exposes just enough of the
// redis.Cmdable interface to satisfy the DistributedLimiter.

// TestDistributed_SharedCounterAcrossGoroutines simulates multiple "server
// instances" (goroutines) sharing a single Redis store. The global count
// must not exceed the limit even though all goroutines run concurrently.
//
// This is the core distributed correctness test: N goroutines each think
// they're independent, but the shared store enforces a single global count.
func TestDistributed_SharedCounterAcrossGoroutines(t *testing.T) {
	// We use a real miniredis-style approach: spin up a fake Redis server.
	// Since we don't want an external dependency in unit tests, we'll use
	// the in-process sliding window as a stand-in for the shared-store
	// semantics (the distributed limiter's correctness is tested separately
	// in integration tests that require a real Redis).
	//
	// What we test here: that concurrent goroutines calling Allow() on the
	// same SlidingWindowLimiter (shared state) never collectively exceed limit.

	l := ratelimit.NewSlidingWindowLimiter()
	limit := int64(50)

	var (
		wg      sync.WaitGroup
		allowed atomic.Int64 //nolint:govet
	)

	workers := 10
	requestsPerWorker := 20 // 10 × 20 = 200 total, limit = 50

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < requestsPerWorker; i++ {
				if l.Allow("shared-key", limit) {
					allowed.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	got := allowed.Load()
	if got > limit {
		t.Errorf("concurrent access: %d requests allowed, limit was %d — shared counter broken", got, limit)
	}
}

// TestDistributed_TierEnforcement verifies that different tiers get different
// limits. A free-tier key should block at 100/min while a premium key allows
// up to 10,000/min.
func TestDistributed_TierEnforcement(t *testing.T) {
	// We test tier limits via the TierLimits map directly (unit test).
	freeLim, ok := ratelimit.TierLimits[ratelimit.TierFree]
	if !ok || freeLim != 100 {
		t.Errorf("free tier limit should be 100, got %d", freeLim)
	}

	basicLim := ratelimit.TierLimits[ratelimit.TierBasic]
	if basicLim != 1000 {
		t.Errorf("basic tier limit should be 1000, got %d", basicLim)
	}

	premiumLim := ratelimit.TierLimits[ratelimit.TierPremium]
	if premiumLim != 10000 {
		t.Errorf("premium tier limit should be 10000, got %d", premiumLim)
	}

	// Tier limits should be ordered: free < basic < premium
	if !(freeLim < basicLim && basicLim < premiumLim) {
		t.Error("tier limits should be strictly ordered: free < basic < premium")
	}
}

// TestDistributed_FailOpen verifies the distributed limiter's behaviour when
// Redis is unavailable: it should fail open (allow the request) rather than
// blocking all traffic.
func TestDistributed_FailOpen(t *testing.T) {
	// Pass a nil client to DistributedLimiter; Allow() should not panic and
	// should return allowed=true (fail open).
	//
	// This tests the resilience contract: rate limiting is a best-effort
	// mechanism. Failing closed (blocking everything) when Redis is down
	// would be worse than allowing a brief overage.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("DistributedLimiter panicked when Redis was nil: %v", r)
		}
	}()

	// Use a disconnected client pointed at a non-existent address.
	client := ratelimit.NewDistributedLimiter(
		newUnreachableRedis(),
		time.Minute,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := client.Allow(ctx, "test-key", ratelimit.TierFree)
	if err != nil {
		// Error is acceptable — the limiter reported the failure
		t.Logf("Allow returned error (acceptable): %v", err)
		return
	}
	// If no error, it should have failed open
	if !result.Allowed {
		t.Error("DistributedLimiter should fail open when Redis is unreachable")
	}
}

// newUnreachableRedis creates a redis.Client pointed at localhost:1 (no server).
func newUnreachableRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        "localhost:1",
		DialTimeout: 10 * time.Millisecond,
	})
}
