package cron

import (
	"sync"
	"time"
)

// LeaderLease is the lease record stored in the shared lease store.
// HolderID is the node that currently holds the lease.
// ExpiresAt is when the lease expires if not renewed.
type LeaderLease struct {
	HolderID  string
	ExpiresAt time.Time
}

// LeaderElector provides in-process leader election via a compare-and-swap
// on a shared LeaderLease. In production, the lease store would be etcd or Redis.
//
// All methods are safe for concurrent use.
type LeaderElector struct {
	mu    sync.Mutex
	lease LeaderLease
}

// NewLeaderElector returns a LeaderElector with no current lease holder.
func NewLeaderElector() *LeaderElector {
	return &LeaderElector{}
}

// Acquire attempts to acquire the lease for nodeID with the given TTL.
// Returns true if the lease was acquired (or was already held by nodeID
// and is still valid).
//
// The CAS semantics:
//   - If the lease is expired (or never set), nodeID takes it.
//   - If the lease is held by nodeID and not expired, it is renewed.
//   - If the lease is held by another node and not expired, returns false.
func (le *LeaderElector) Acquire(nodeID string, ttl time.Duration) bool {
	le.mu.Lock()
	defer le.mu.Unlock()
	now := time.Now()
	if le.lease.HolderID == "" || now.After(le.lease.ExpiresAt) {
		// Lease is free — take it.
		le.lease = LeaderLease{
			HolderID:  nodeID,
			ExpiresAt: now.Add(ttl),
		}
		return true
	}
	if le.lease.HolderID == nodeID {
		// Already held by us — renew in-place.
		le.lease.ExpiresAt = now.Add(ttl)
		return true
	}
	return false
}

// Renew extends the lease for nodeID by ttl. Returns true if the lease was
// renewed (nodeID is still the current holder and the lease has not expired).
func (le *LeaderElector) Renew(nodeID string, ttl time.Duration) bool {
	le.mu.Lock()
	defer le.mu.Unlock()
	now := time.Now()
	if le.lease.HolderID == nodeID && now.Before(le.lease.ExpiresAt) {
		le.lease.ExpiresAt = now.Add(ttl)
		return true
	}
	return false
}

// IsLeader returns true if nodeID currently holds a valid lease.
func (le *LeaderElector) IsLeader(nodeID string) bool {
	le.mu.Lock()
	defer le.mu.Unlock()
	return le.lease.HolderID == nodeID && time.Now().Before(le.lease.ExpiresAt)
}

// CurrentLease returns a snapshot of the current lease (for observability).
func (le *LeaderElector) CurrentLease() LeaderLease {
	le.mu.Lock()
	defer le.mu.Unlock()
	return le.lease
}

// Release explicitly releases the lease if held by nodeID.
// Used for clean shutdown — in production, let the TTL expire instead.
func (le *LeaderElector) Release(nodeID string) bool {
	le.mu.Lock()
	defer le.mu.Unlock()
	if le.lease.HolderID == nodeID {
		le.lease = LeaderLease{}
		return true
	}
	return false
}
