package cdn

import (
	"sync/atomic"
)

// CacheStats holds atomic counters for the two-level cache.
// All fields are updated with sync/atomic; no mutex needed for increments.
type CacheStats struct {
	L1Hits       atomic.Int64
	L2Hits       atomic.Int64
	Misses       atomic.Int64
	BytesCached  atomic.Int64
	EvictionCount atomic.Int64
}

// HitRatio returns the fraction of requests served from cache (L1 + L2 hits
// over total requests). Returns 0 if no requests have been made yet.
func (s *CacheStats) HitRatio() float64 {
	l1 := s.L1Hits.Load()
	l2 := s.L2Hits.Load()
	misses := s.Misses.Load()
	total := l1 + l2 + misses
	if total == 0 {
		return 0
	}
	return float64(l1+l2) / float64(total)
}

// Snapshot returns a point-in-time copy of the stats (safe for JSON encoding).
func (s *CacheStats) Snapshot() StatsSnapshot {
	l1 := s.L1Hits.Load()
	l2 := s.L2Hits.Load()
	misses := s.Misses.Load()
	total := l1 + l2 + misses
	ratio := 0.0
	if total > 0 {
		ratio = float64(l1+l2) / float64(total)
	}
	return StatsSnapshot{
		L1Hits:        l1,
		L2Hits:        l2,
		Misses:        misses,
		BytesCached:   s.BytesCached.Load(),
		EvictionCount: s.EvictionCount.Load(),
		HitRatio:      ratio,
	}
}

// StatsSnapshot is an immutable copy of CacheStats for serialization.
type StatsSnapshot struct {
	L1Hits        int64   `json:"l1_hits"`
	L2Hits        int64   `json:"l2_hits"`
	Misses        int64   `json:"misses"`
	BytesCached   int64   `json:"bytes_cached"`
	EvictionCount int64   `json:"eviction_count"`
	HitRatio      float64 `json:"hit_ratio"`
}
