// Package ratelimit provides three rate limiting implementations:
//
//   - v0: In-process token bucket (burst-friendly, O(1) per key)
//   - v1: Sliding window counter using an atomic ring buffer (no boundary bursts)
//   - v2: Distributed rate limiter backed by a Redis-compatible store
//
// Each stage illustrates a distinct tradeoff. The token bucket is fastest but
// allows bursts. The sliding window removes boundary bursts but is still
// in-process (N servers = N× the global limit). The distributed store gives
// accurate global counts at the cost of one network round-trip per check.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------
// v0: Token bucket — bursts allowed, average rate enforced
// ---------------------------------------------------------------------------

// TokenBucket holds the state for a single key's token bucket.
// The bucket fills at refillRate tokens/sec, up to capacity tokens.
// Each allowed request consumes exactly 1 token.
//
// Key lesson: the bucket allows a burst of up to `capacity` requests
// (when the bucket is full), but the long-term average is capped at
// `refillRate` requests/sec. This is different from a leaky bucket,
// which smooths bursts by draining at a constant rate regardless of
// arrival pattern.
type TokenBucket struct {
	capacity   float64
	refillRate float64 // tokens per second
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a full bucket with the given capacity and refill rate.
func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		refillRate: refillRate,
		tokens:     capacity, // start full — allow an immediate burst
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is permitted and consumes one token if so.
// Thread-safe.
func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.lastRefill = now

	// Refill: add tokens earned since the last call, capped at capacity.
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	if b.tokens < 1.0 {
		return false // bucket empty — reject
	}
	b.tokens -= 1.0
	return true
}

// Tokens returns the current token count (for observability / tests).
func (b *TokenBucket) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
}

// TokenBucketLimiter manages a map of per-key token buckets.
type TokenBucketLimiter struct {
	capacity   float64
	refillRate float64
	buckets    map[string]*TokenBucket
	mu         sync.Mutex
}

// NewTokenBucketLimiter creates a limiter where every key shares the same
// capacity and refill rate.
func NewTokenBucketLimiter(capacity float64, refillRate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		capacity:   capacity,
		refillRate: refillRate,
		buckets:    make(map[string]*TokenBucket),
	}
}

// Allow returns true if the key has a token available.
func (l *TokenBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	b, ok := l.buckets[key]
	if !ok {
		b = NewTokenBucket(l.capacity, l.refillRate)
		l.buckets[key] = b
	}
	l.mu.Unlock()
	return b.Allow()
}

// BucketFor returns the underlying token bucket for a key (creates it if absent).
// Used by tests and the /status endpoint.
func (l *TokenBucketLimiter) BucketFor(key string) *TokenBucket {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = NewTokenBucket(l.capacity, l.refillRate)
		l.buckets[key] = b
	}
	return b
}

// ---------------------------------------------------------------------------
// v1: Sliding window counter — no boundary bursts
// ---------------------------------------------------------------------------

// windowSize is the number of 1-second buckets in the ring. A 60-second
// sliding window means we sum the last 60 one-second counters.
const windowSize = 60

// slidingBucket holds one second's worth of request counts.
// We pack both the timestamp (which second this bucket represents) and
// the count into two separate atomic int64 values.
type slidingBucket struct {
	ts    atomic.Int64 // Unix second when this bucket was last reset
	count atomic.Int64 // requests in that second
}

// SlidingWindowBuckets is the per-key ring buffer for v1.
type SlidingWindowBuckets struct {
	ring [windowSize]slidingBucket
}

// Add increments the counter for the current second, resetting the slot if
// it belongs to a different second. Returns the updated count.
// Lock-free: each slot is owned by exactly one Unix-second.
func (s *SlidingWindowBuckets) Add() {
	now := time.Now().Unix()
	slot := now % windowSize

	ts := s.ring[slot].ts.Load()
	if ts != now {
		// This slot is from a previous cycle — reset it atomically.
		// A CAS prevents two goroutines from both resetting.
		if s.ring[slot].ts.CompareAndSwap(ts, now) {
			s.ring[slot].count.Store(0)
		}
	}
	s.ring[slot].count.Add(1)
}

// Sum counts requests in the last windowSize seconds.
func (s *SlidingWindowBuckets) Sum() int64 {
	now := time.Now().Unix()
	var total int64
	for i := 0; i < windowSize; i++ {
		ts := s.ring[i].ts.Load()
		// Only count buckets that belong to the current window.
		if now-ts < windowSize {
			total += s.ring[i].count.Load()
		}
	}
	return total
}

// SlidingWindowLimiter manages per-key sliding window counters.
//
// Key lesson: fixed-window rate limiting has a boundary problem.
// If the limit is 100/min and the window resets at T=60.0, a client can
// send 100 requests at T=59.9 (within window 1) and 100 more at T=60.1
// (within window 2). In 0.2 seconds, 200 requests passed — 2x the limit.
// The sliding window doesn't have this flaw: it always sums the last 60
// one-second buckets, regardless of clock alignment.
type SlidingWindowLimiter struct {
	buckets map[string]*SlidingWindowBuckets
	mu      sync.RWMutex
}

// NewSlidingWindowLimiter creates a sliding-window limiter.
func NewSlidingWindowLimiter() *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		buckets: make(map[string]*SlidingWindowBuckets),
	}
}

// Allow returns true and records the request if the key is under limit.
func (l *SlidingWindowLimiter) Allow(key string, limit int64) bool {
	b := l.bucketsFor(key)
	current := b.Sum()
	if current >= limit {
		return false
	}
	b.Add()
	return true
}

// Count returns the current request count in the sliding window (without
// recording a new request). Used by tests and the /status endpoint.
func (l *SlidingWindowLimiter) Count(key string) int64 {
	return l.bucketsFor(key).Sum()
}

func (l *SlidingWindowLimiter) bucketsFor(key string) *SlidingWindowBuckets {
	l.mu.RLock()
	b, ok := l.buckets[key]
	l.mu.RUnlock()
	if ok {
		return b
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// Double-check after acquiring write lock.
	if b, ok = l.buckets[key]; ok {
		return b
	}
	b = &SlidingWindowBuckets{}
	l.buckets[key] = b
	return b
}

// ---------------------------------------------------------------------------
// v2: Distributed rate limiter — Redis INCR + EXPIRE
// ---------------------------------------------------------------------------

// Tier represents a named service tier with its own rate limit.
// The same algorithm runs for every tier; only the limit changes.
//
// Key lesson: N servers × in-process limit = N× the global limit.
// With 10 servers each allowing 100 req/min, the effective limit is
// 1000 req/min. For billing-sensitive limits or abuse prevention that's
// unacceptable. A shared store (Redis) gives a single global counter
// that all server instances update atomically.
type Tier string

const (
	TierFree    Tier = "free"    // 100 requests/minute
	TierBasic   Tier = "basic"   // 1000 requests/minute
	TierPremium Tier = "premium" // 10000 requests/minute
)

// TierLimits maps each tier to its per-minute request limit.
var TierLimits = map[Tier]int64{
	TierFree:    100,
	TierBasic:   1000,
	TierPremium: 10000,
}

// RateLimitResult is the outcome of a distributed rate limit check.
type RateLimitResult struct {
	Allowed   bool
	Remaining int64
	ResetAt   time.Time
}

// DistributedLimiter uses a Redis-compatible store to enforce global limits
// across multiple server instances.
//
// The atomic sequence per check:
//  1. INCR key           — atomically increment the counter
//  2. If count == 1, EXPIRE key window — set TTL on first increment only
//
// This is executed in a Redis pipeline (both commands in one round-trip).
// The INCR is atomic in Redis even under concurrent access from N clients,
// so the global count is always accurate within the TTL window.
//
// Approximate accuracy note: under network partitions, two Redis replicas can
// both serve requests up to the limit, causing ~2x overage. Production systems
// (Stripe, Cloudflare) accept this ~1% overage to avoid the latency of
// distributed consensus. Our toy uses a single Redis node, so it's exact.
type DistributedLimiter struct {
	client redis.Cmdable
	window time.Duration
}

// NewDistributedLimiter creates a limiter backed by the given Redis client.
// window is the rate limit window (e.g., 1 * time.Minute for per-minute limits).
func NewDistributedLimiter(client redis.Cmdable, window time.Duration) *DistributedLimiter {
	return &DistributedLimiter{client: client, window: window}
}

// Allow checks whether key is under the given limit for its tier, and
// atomically records the request if permitted.
func (d *DistributedLimiter) Allow(ctx context.Context, key string, tier Tier) (*RateLimitResult, error) {
	limit, ok := TierLimits[tier]
	if !ok {
		limit = TierLimits[TierFree] // default to most restrictive
	}

	redisKey := fmt.Sprintf("rl:%s:%s:%d", key, tier, time.Now().Unix()/int64(d.window.Seconds()))

	pipe := d.client.Pipeline()
	incrCmd := pipe.Incr(ctx, redisKey)
	pipe.Expire(ctx, redisKey, d.window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		// Pipeline error — fail open (allow the request) to avoid blocking
		// legitimate traffic when Redis is briefly unavailable.
		return &RateLimitResult{Allowed: true, Remaining: -1, ResetAt: time.Now().Add(d.window)}, nil
	}

	count := incrCmd.Val()
	resetAt := time.Now().Truncate(d.window).Add(d.window)

	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return &RateLimitResult{
		Allowed:   count <= limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

// Count returns the current request count for a key+tier without incrementing.
// Used by the /status endpoint.
func (d *DistributedLimiter) Count(ctx context.Context, key string, tier Tier) (int64, error) {
	redisKey := fmt.Sprintf("rl:%s:%s:%d", key, tier, time.Now().Unix()/int64(d.window.Seconds()))
	val, err := d.client.Get(ctx, redisKey).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}
