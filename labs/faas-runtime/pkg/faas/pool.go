package faas

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// coldStartDelay simulates the time required to start a new container
	// or microVM. Real Lambda cold starts range from 100ms (Node.js) to
	// 500ms+ (JVM without SnapStart).
	coldStartDelay = 50 * time.Millisecond

	// defaultMaxWarm is the default number of warm instances kept per function.
	defaultMaxWarm = 3

	// defaultWarmTimeout is how long an idle instance is kept before eviction.
	defaultWarmTimeout = 5 * time.Minute
)

// Instance represents a single runtime slot for a function — conceptually a
// lightweight process or container that has been initialised and is ready to
// handle an invocation without paying the cold-start penalty.
//
// In our simulation an Instance is just metadata; in a real FaaS runtime it
// would hold a process handle, a UNIX socket to the worker, and a network
// namespace file descriptor.
type Instance struct {
	function *Function
	lastUsed time.Time
	busy     atomic.Bool
}

// InvocationStats tracks aggregate counts across all invocations.
// Fields are updated atomically so Stats() is safe to call concurrently.
type InvocationStats struct {
	ColdStarts int64
	WarmHits   int64
	Timeouts   int64
	Panics     int64
}

func (s *InvocationStats) addCold()    { atomic.AddInt64(&s.ColdStarts, 1) }
func (s *InvocationStats) addWarm()    { atomic.AddInt64(&s.WarmHits, 1) }
func (s *InvocationStats) addTimeout() { atomic.AddInt64(&s.Timeouts, 1) }
func (s *InvocationStats) addPanic()   { atomic.AddInt64(&s.Panics, 1) }

// Snapshot returns a copy of the current stats.
func (s *InvocationStats) Snapshot() InvocationStats {
	return InvocationStats{
		ColdStarts: atomic.LoadInt64(&s.ColdStarts),
		WarmHits:   atomic.LoadInt64(&s.WarmHits),
		Timeouts:   atomic.LoadInt64(&s.Timeouts),
		Panics:     atomic.LoadInt64(&s.Panics),
	}
}

// InstancePool manages warm instance pools for multiple functions.
// Each function gets its own slice of idle instances, capped at maxWarm.
type InstancePool struct {
	mu          sync.Mutex
	pools       map[string][]*Instance // function name -> idle instances
	functions   map[string]*Function   // function name -> Function definition
	maxWarm     int
	warmTimeout time.Duration
	stats       InvocationStats

	stopEvict chan struct{}
}

// NewInstancePool creates a pool with the given maximum warm instances per function.
func NewInstancePool(maxWarm int) *InstancePool {
	if maxWarm <= 0 {
		maxWarm = defaultMaxWarm
	}
	p := &InstancePool{
		pools:       make(map[string][]*Instance),
		functions:   make(map[string]*Function),
		maxWarm:     maxWarm,
		warmTimeout: defaultWarmTimeout,
		stopEvict:   make(chan struct{}),
	}
	go p.evictLoop()
	return p
}

// Register tells the pool that a function exists. Called by Runtime.Register.
func (p *InstancePool) Register(name string) {
	p.mu.Lock()
	if _, ok := p.pools[name]; !ok {
		p.pools[name] = make([]*Instance, 0, p.maxWarm)
	}
	p.mu.Unlock()
}

// Acquire returns a ready Instance for the named function.
//
// Algorithm:
//  1. Lock the pool; scan the idle list for an instance where busy == false.
//  2. If found: mark busy, update lastUsed, increment WarmHits, return.
//  3. If not found: release the lock, sleep 50ms (cold start), allocate a new
//     Instance, mark busy, increment ColdStarts, return.
//
// The 50ms cold start delay is incurred outside the pool lock so concurrent
// cold starts do not serialise each other.
func (p *InstancePool) Acquire(name string) (*Instance, error) {
	p.mu.Lock()
	idle := p.pools[name]
	for _, inst := range idle {
		if inst.busy.CompareAndSwap(false, true) {
			inst.lastUsed = time.Now()
			p.mu.Unlock()
			p.stats.addWarm()
			return inst, nil
		}
	}
	p.mu.Unlock()

	// Cold start: simulate container/microVM startup.
	time.Sleep(coldStartDelay)
	p.stats.addCold()

	inst := &Instance{
		lastUsed: time.Now(),
	}
	inst.busy.Store(true)
	return inst, nil
}

// AcquireWithSnapshot is like Acquire but tries to restore from a snapshot
// (5ms) instead of paying the full cold start penalty (50ms) when no warm
// instance is available.
func (p *InstancePool) AcquireWithSnapshot(name string, store *SnapshotStore) (*Instance, error) {
	p.mu.Lock()
	idle := p.pools[name]
	for _, inst := range idle {
		if inst.busy.CompareAndSwap(false, true) {
			inst.lastUsed = time.Now()
			p.mu.Unlock()
			p.stats.addWarm()
			return inst, nil
		}
	}
	p.mu.Unlock()

	// Try snapshot restore before falling back to full cold start.
	if store.Restore(name) {
		p.stats.addCold()
		inst := &Instance{lastUsed: time.Now()}
		inst.busy.Store(true)
		return inst, nil
	}

	// No snapshot: full cold start.
	time.Sleep(coldStartDelay)
	// Save a snapshot for next time.
	store.Save(name, []byte("initialized"))
	p.stats.addCold()

	inst := &Instance{lastUsed: time.Now()}
	inst.busy.Store(true)
	return inst, nil
}

// Release returns inst to the warm pool after an invocation completes.
// If the pool is already at maxWarm idle instances, the instance is discarded.
func (p *InstancePool) Release(name string, inst *Instance) {
	inst.busy.Store(false)
	inst.lastUsed = time.Now()

	p.mu.Lock()
	defer p.mu.Unlock()

	idle := p.pools[name]
	// Count how many are currently not busy.
	idleCount := 0
	for _, i := range idle {
		if !i.busy.Load() {
			idleCount++
		}
	}
	if idleCount < p.maxWarm {
		p.pools[name] = append(idle, inst)
	}
	// Otherwise discard: the instance is not added to the pool.
}

// Evict removes instances that have been idle longer than warmTimeout.
// Called by the background eviction goroutine.
func (p *InstancePool) Evict() {
	cutoff := time.Now().Add(-p.warmTimeout)
	p.mu.Lock()
	defer p.mu.Unlock()

	for name, idle := range p.pools {
		var keep []*Instance
		for _, inst := range idle {
			if inst.busy.Load() || inst.lastUsed.After(cutoff) {
				keep = append(keep, inst)
			}
		}
		p.pools[name] = keep
	}
}

// evictLoop runs Evict periodically until Close() is called.
func (p *InstancePool) evictLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.Evict()
		case <-p.stopEvict:
			return
		}
	}
}

// Close shuts down the background eviction goroutine.
func (p *InstancePool) Close() {
	close(p.stopEvict)
}

// Stats returns a copy of the current invocation statistics.
func (p *InstancePool) Stats() *InvocationStats {
	snap := p.stats.Snapshot()
	return &snap
}

// IdleCount returns the number of idle (non-busy) instances in the named pool.
// Used by tests to verify pool cap behaviour.
func (p *InstancePool) IdleCount(name string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, inst := range p.pools[name] {
		if !inst.busy.Load() {
			count++
		}
	}
	return count
}
