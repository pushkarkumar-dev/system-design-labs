// Package lock provides three distributed lock implementations:
//
//   - v0: In-process lease-based lock manager with automatic TTL expiry
//   - v1: Fencing token lock — monotonically increasing token prevents stale writers
//   - v2: Distributed lock via atomic SET NX PX semantics (Redis-compatible)
//
// Each stage illustrates a distinct correctness property. The in-process lock
// demonstrates leases. The fencing token demonstrates how to make storage
// servers reject stale lock holders. The distributed lock demonstrates why
// the only correct primitive is a single atomic conditional write.
package lock

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------
// v0: Lease-based in-process lock manager
// ---------------------------------------------------------------------------

// LockEntry holds the state for a single resource's lock.
// The key lesson: expiresAt makes this a *lease*, not a plain lock.
// If the holder crashes without calling Release, the lock self-heals
// after TTL elapses — no deadlock possible.
type LockEntry struct {
	owner     string
	token     int64
	expiresAt time.Time
}

// IsExpired returns true if the lock's TTL has elapsed.
func (e *LockEntry) IsExpired() bool {
	return time.Now().After(e.expiresAt)
}

// LockManager manages leases for multiple resources.
// It is safe for concurrent use; every public method acquires mu.
//
// Key design: the token is a monotonically increasing int64 (not random).
// This enables fencing (see v1): a storage server can reject any write with
// a token older than the highest token it has ever seen.
type LockManager struct {
	mu      sync.Mutex
	entries map[string]*LockEntry
	counter int64 // global fencing counter — incremented on every Acquire/Renew
}

// NewLockManager creates an empty lock manager ready to use.
func NewLockManager() *LockManager {
	return &LockManager{
		entries: make(map[string]*LockEntry),
	}
}

// Acquire attempts to take the lock on resource for the given owner with ttl.
// Returns (token, true) if the lock was granted.
// Returns (0, false) if the resource is already locked by another owner and has not expired.
//
// If the existing lock has expired, it is silently replaced — this is the
// self-healing property of leases.
func (m *LockManager) Acquire(resource, owner string, ttl time.Duration) (int64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.entries[resource]
	if ok && !existing.IsExpired() {
		// Lock is held by someone else and has not expired.
		return 0, false
	}

	// Generate next fencing token. Monotonically increasing — never reused.
	token := atomic.AddInt64(&m.counter, 1)
	m.entries[resource] = &LockEntry{
		owner:     owner,
		token:     token,
		expiresAt: time.Now().Add(ttl),
	}
	return token, true
}

// Release frees the lock on resource only if token matches the current holder's token.
// Returns true if the lock was released.
// Returns false if the token is wrong (stale release attempt after the lock expired
// and was re-acquired by a new holder).
func (m *LockManager) Release(resource string, token int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[resource]
	if !ok {
		return false
	}
	if entry.token != token {
		return false // stale token — do not release the new holder's lock
	}
	delete(m.entries, resource)
	return true
}

// Renew extends the TTL of an existing lock if owner and token match.
// Returns false if the lock has expired or the token does not match.
//
// The renewal pattern: acquire with a short TTL (e.g. 5s), then call Renew
// every 2s as long as the critical section is running. If the process crashes,
// renewal stops and the lock self-heals after the TTL.
func (m *LockManager) Renew(resource, owner string, token int64, ttl time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[resource]
	if !ok {
		return false
	}
	if entry.token != token || entry.owner != owner {
		return false
	}
	if entry.IsExpired() {
		return false // too late — the lease has lapsed
	}
	entry.expiresAt = time.Now().Add(ttl)
	return true
}

// State returns a snapshot of the current lock state for a resource.
// Returns nil if the resource is not locked or its lock has expired.
func (m *LockManager) State(resource string) *LockSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[resource]
	if !ok || entry.IsExpired() {
		return nil
	}
	return &LockSnapshot{
		Owner:     entry.owner,
		Token:     entry.token,
		ExpiresAt: entry.expiresAt,
		TTLLeft:   time.Until(entry.expiresAt),
	}
}

// LockSnapshot is a point-in-time view of a lock's state.
type LockSnapshot struct {
	Owner     string
	Token     int64
	ExpiresAt time.Time
	TTLLeft   time.Duration
}

// ---------------------------------------------------------------------------
// v1: Fencing token — storage server rejects stale writers
// ---------------------------------------------------------------------------

// ErrStaleFencingToken is returned when a write is attempted with a token
// lower than the last accepted token. This is the key correctness guarantee:
// even if a stale process wakes up from a GC pause still holding its old
// lock, the storage server rejects its writes.
var ErrStaleFencingToken = errors.New("stale fencing token: write rejected")

// StorageServer simulates a resource that enforces fencing tokens.
// Every call to Write must pass a token at least as large as the last
// accepted token. Stale tokens (from processes whose locks have expired)
// are rejected outright.
//
// In a real system this is a database, an S3 conditional PUT, or any
// resource that can store and compare a version number.
type StorageServer struct {
	mu        sync.Mutex
	lastToken int64
	data      map[string]storageEntry
}

type storageEntry struct {
	value string
	token int64
}

// NewStorageServer creates an empty storage server ready for writes.
func NewStorageServer() *StorageServer {
	return &StorageServer{
		data: make(map[string]storageEntry),
	}
}

// Write stores key=value only if token >= the last accepted token.
// Rejects writes with a lower token — the stale lock holder is not allowed to write.
func (s *StorageServer) Write(key, value string, token int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token < s.lastToken {
		return fmt.Errorf("%w: got %d, last accepted %d", ErrStaleFencingToken, token, s.lastToken)
	}
	s.lastToken = token
	s.data[key] = storageEntry{value: value, token: token}
	return nil
}

// Read returns the current value and last write token for key.
func (s *StorageServer) Read(key string) (string, int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok {
		return "", 0, false
	}
	return e.value, e.token, true
}

// LastToken returns the highest fencing token seen by this storage server.
func (s *StorageServer) LastToken() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastToken
}

// ---------------------------------------------------------------------------
// v2: Distributed lock via atomic SET NX PX (Redis-compatible)
// ---------------------------------------------------------------------------

// ErrLockNotHeld is returned when Release is called but the caller is not the
// current holder (the lock expired and was re-acquired by another process).
var ErrLockNotHeld = errors.New("lock not held: token mismatch or lock expired")

// DistributedLockManager acquires and releases distributed locks using a
// Redis-compatible store with atomic SET NX PX semantics.
//
// The key insight: SET NX PX is a single atomic operation — "set this key to
// this value, but only if it does not already exist, and expire it after N ms".
// There is no window between the existence check and the set. This is the
// only correct primitive for distributed mutual exclusion on a single node.
//
// Why GET-then-SET is broken: between your GET (key not present) and your SET
// (write the key), another process can acquire the lock. You've just granted
// two processes the lock simultaneously. NX prevents this race entirely.
type DistributedLockManager struct {
	client  *redis.Client
	counter int64 // local fencing counter for this process
}

// NewDistributedLockManager creates a lock manager backed by the given Redis client.
func NewDistributedLockManager(client *redis.Client) *DistributedLockManager {
	return &DistributedLockManager{client: client}
}

// lockKey returns the Redis key for a resource.
func lockKey(resource string) string {
	return "lock:" + resource
}

// lockValue encodes owner and token as the Redis value.
// Format: "owner:token" — the owner is verified on release to prevent
// accidental release by a different process.
func lockValue(owner string, token int64) string {
	return fmt.Sprintf("%s:%d", owner, token)
}

// Acquire attempts to acquire the distributed lock on resource.
// Uses SET lock:{resource} {owner}:{token} NX PX {ttl_ms} — atomic.
// Returns (token, true) on success, (0, false) if already held.
func (d *DistributedLockManager) Acquire(ctx context.Context, resource, owner string, ttl time.Duration) (int64, bool, error) {
	token := atomic.AddInt64(&d.counter, 1)
	value := lockValue(owner, token)

	ok, err := d.client.SetNX(ctx, lockKey(resource), value, ttl).Result()
	if err != nil {
		return 0, false, fmt.Errorf("acquire: %w", err)
	}
	if !ok {
		return 0, false, nil // lock is held by another owner
	}
	return token, true, nil
}

// Release frees the lock only if the caller holds it (token matches).
// Uses a pipeline: GET to verify owner+token, then DEL.
//
// This two-step pipeline (not a Lua script, for clarity) has a small TOCTOU
// window in theory — a production implementation should use a Lua script to
// make GET+DEL fully atomic. The key lesson stands: release must verify the
// token before deleting, or a stale process can evict the new holder's lock.
func (d *DistributedLockManager) Release(ctx context.Context, resource string, token int64, owner string) error {
	key := lockKey(resource)
	current, err := d.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return ErrLockNotHeld
	}
	if err != nil {
		return fmt.Errorf("release get: %w", err)
	}

	expected := lockValue(owner, token)
	if current != expected {
		return fmt.Errorf("%w: resource=%s expected=%s got=%s", ErrLockNotHeld, resource, expected, current)
	}

	if err := d.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("release del: %w", err)
	}
	return nil
}

// State returns the current lock state from Redis.
// Returns nil if the resource is not locked.
func (d *DistributedLockManager) State(ctx context.Context, resource string) (*DistributedLockState, error) {
	key := lockKey(resource)
	val, err := d.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state get: %w", err)
	}

	ttl, err := d.client.PTTL(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("state pttl: %w", err)
	}

	return &DistributedLockState{Value: val, TTLLeft: ttl}, nil
}

// DistributedLockState is a snapshot of a Redis-backed lock's state.
type DistributedLockState struct {
	Value   string
	TTLLeft time.Duration
}
