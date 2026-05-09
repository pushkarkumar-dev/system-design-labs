package faas

import (
	"sync"
	"time"
)

const (
	// snapshotRestoreDelay simulates the time to restore a function's state
	// from a snapshot (object storage fetch + deserialization). Real Lambda
	// SnapStart achieves ~100ms restore vs 500ms+ JVM cold start.
	// Our simulation uses 5ms vs 50ms for clarity.
	snapshotRestoreDelay = 5 * time.Millisecond
)

// Snapshot captures the initialized state of a function after its first cold
// start. On subsequent cold starts, the runtime restores from the snapshot
// instead of re-running initialization — 10× faster in our simulation.
//
// Real SnapStart (AWS Lambda) takes a memory snapshot of the JVM process after
// the init() phase, stores it in S3, and restores it via UFFD (userfaultfd)
// page-fault demand-paging on the next cold start.
type Snapshot struct {
	// FuncName is the function this snapshot belongs to.
	FuncName string

	// InitState is the serialized initialized state of the function.
	// In our simulation this is a simple byte slice written during the first
	// cold start. Real snapshots are full process memory images.
	InitState []byte

	// CreatedAt records when the snapshot was first taken.
	CreatedAt time.Time

	// RestoredCount tracks how many times this snapshot has been used to
	// avoid a full cold start.
	RestoredCount int
}

// SnapshotStore is an in-memory store for function snapshots, keyed by
// function name. In production this would be backed by object storage (S3).
type SnapshotStore struct {
	mu        sync.Mutex
	snapshots map[string]*Snapshot
}

// NewSnapshotStore creates an empty SnapshotStore.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{
		snapshots: make(map[string]*Snapshot),
	}
}

// Save stores a snapshot for the named function. If a snapshot already exists
// it is replaced (re-snapshotting after a code update).
func (s *SnapshotStore) Save(name string, initState []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[name] = &Snapshot{
		FuncName:  name,
		InitState: initState,
		CreatedAt: time.Now(),
	}
}

// Restore attempts to restore a function from its snapshot.
// If a snapshot exists, it sleeps snapshotRestoreDelay (5ms) to simulate the
// restore latency, increments RestoredCount, and returns true.
// If no snapshot exists, it returns false without sleeping.
func (s *SnapshotStore) Restore(name string) bool {
	s.mu.Lock()
	snap, ok := s.snapshots[name]
	if ok {
		snap.RestoredCount++
	}
	s.mu.Unlock()

	if !ok {
		return false
	}

	// Simulate snapshot restore latency (5ms — 10× faster than cold start).
	time.Sleep(snapshotRestoreDelay)
	return true
}

// Get returns the snapshot for the named function, or nil if none exists.
func (s *SnapshotStore) Get(name string) *Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshots[name]
}

// Delete removes the snapshot for the named function. Called when a function
// is updated — the old snapshot is stale.
func (s *SnapshotStore) Delete(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, name)
}
