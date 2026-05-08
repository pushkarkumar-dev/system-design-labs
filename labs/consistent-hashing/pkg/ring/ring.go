// Package ring implements a consistent hashing ring.
//
// Three progressive versions live in this file:
//
//   v0 — Basic ring: MD5 hash, sorted positions, clockwise successor lookup.
//        The simplest structure that makes the algorithm visible.
//
//   v1 — Virtual nodes: each physical node gets 100 positions on the ring.
//        Standard deviation of key distribution drops from ~28% to ~4%.
//
//   v2 — Dynamic membership: add/remove nodes with minimal key remapping.
//        Reports remapping stats so you can verify the 1/N theoretical bound.
//
// The key invariant in all three: given a key, find the node whose ring
// position is the smallest value ≥ hash(key), wrapping around to the first
// node if no such position exists (clockwise successor).

package ring

import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
)

// ── Shared types ────────────────────────────────────────────────────────────

// Node represents a physical server on the ring.
type Node struct {
	Name string
	Addr string
}

// RemapStats is returned by AddNode / RemoveNode to show how many keys
// were moved and whether it matches the 1/N theoretical minimum.
type RemapStats struct {
	TotalKeys    int
	RemappedKeys int
	RemapRate    float64 // fraction in [0,1]
	Theoretical  float64 // 1 / (old node count)  for add,  1 / (old node count) for remove
}

// DistributionStats reports how evenly keys are spread across nodes.
type DistributionStats struct {
	KeysPerNode map[string]int
	StdDev      float64 // as a fraction of the mean (coefficient of variation)
	Min         int
	Max         int
}

// ── v0 — Basic ring (MD5, sorted positions, clockwise scan) ─────────────────
//
// Lesson: modular hashing (key % N) remaps ALL keys when N changes.
// The ring fix: both keys and nodes live in the same 32-bit space [0, 2^32).
// A key belongs to the node at the next clockwise position.  Adding one node
// only steals keys from that one node's predecessor range — nothing else moves.

// HashRing is the v0 ring with one position per physical node.
type HashRing struct {
	mu        sync.RWMutex
	positions []uint32          // sorted ring positions
	nodeMap   map[uint32]*Node  // position → node
	nodes     map[string]*Node  // name → node (for membership checks)
	vnodes    int               // virtual nodes per physical node (1 for v0)
}

// New creates a v0 ring — one position per physical node.
func New() *HashRing {
	return &HashRing{
		nodeMap: make(map[uint32]*Node),
		nodes:   make(map[string]*Node),
		vnodes:  1,
	}
}

// hash32 returns the first 4 bytes of the MD5 digest of the input, interpreted
// as a big-endian uint32. This maps any string to [0, 2^32).
func hash32(key string) uint32 {
	sum := md5.Sum([]byte(key))
	return binary.BigEndian.Uint32(sum[:4])
}

// AddNode inserts a physical node at its MD5-derived ring position.
// If the node is already present, AddNode is a no-op.
func (r *HashRing) AddNode(n Node) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[n.Name]; exists {
		return
	}

	node := &Node{Name: n.Name, Addr: n.Addr}
	r.nodes[n.Name] = node

	pos := hash32(n.Name)
	r.positions = append(r.positions, pos)
	r.nodeMap[pos] = node

	sort.Slice(r.positions, func(i, j int) bool {
		return r.positions[i] < r.positions[j]
	})
}

// RemoveNode removes a node from the ring.  Its keys migrate to the clockwise
// successor — exactly the minimum movement required.
func (r *HashRing) RemoveNode(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[name]; !exists {
		return
	}

	pos := hash32(name)
	delete(r.nodeMap, pos)
	delete(r.nodes, name)

	idx := sort.Search(len(r.positions), func(i int) bool {
		return r.positions[i] >= pos
	})
	if idx < len(r.positions) {
		r.positions = append(r.positions[:idx], r.positions[idx+1:]...)
	}
}

// GetNode returns the node responsible for the given key.
// It hashes the key and finds the clockwise successor on the ring.
// If no nodes are present, GetNode returns nil.
func (r *HashRing) GetNode(key string) *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.positions) == 0 {
		return nil
	}

	h := hash32(key)

	// Binary search for the first position >= h
	idx := sort.Search(len(r.positions), func(i int) bool {
		return r.positions[i] >= h
	})

	// Wrap around: if h is beyond the last position, use the first node
	if idx == len(r.positions) {
		idx = 0
	}

	return r.nodeMap[r.positions[idx]]
}

// NodeCount returns the number of physical nodes on the ring.
func (r *HashRing) NodeCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

// Nodes returns a snapshot of all physical nodes.
func (r *HashRing) Nodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		result = append(result, *n)
	}
	return result
}

// ── v1 — Virtual nodes (100 vNodes per physical node) ───────────────────────
//
// Lesson: with 5 physical nodes and 1 position each, one node can get 31% of
// keys and another only 9% — pure accident of MD5 hash collisions in the ring
// space. The fix: each physical node occupies 100 positions on the ring,
// labelled "name#0", "name#1", ..., "name#99". The more positions, the more
// evenly load is distributed. Standard deviation tracks as ~1/sqrt(vNodes).
//
// Cost: the ring is 100× larger (still tiny — 5 nodes × 100 = 500 entries).

// VNodeRing is the v1 ring with configurable virtual nodes per physical node.
type VNodeRing struct {
	mu        sync.RWMutex
	positions []uint32         // sorted ring positions
	nodeMap   map[uint32]*Node // vnode position → physical node
	nodes     map[string]*Node // name → physical node
	vnodesN   int              // virtual nodes per physical node
}

// NewVNode creates a v1 ring with the specified number of virtual nodes per
// physical node.  vnodesN=100 gives a good balance of uniformity and memory.
func NewVNode(vnodesN int) *VNodeRing {
	if vnodesN < 1 {
		vnodesN = 1
	}
	return &VNodeRing{
		nodeMap: make(map[uint32]*Node),
		nodes:   make(map[string]*Node),
		vnodesN: vnodesN,
	}
}

// vnodeKey returns the label for the i-th virtual node of a physical node.
// Example: vnodeKey("cache-1", 7) → "cache-1#7"
func vnodeKey(name string, i int) string {
	return fmt.Sprintf("%s#%d", name, i)
}

// AddNode places the physical node at vnodesN positions on the ring.
func (r *VNodeRing) AddNode(n Node) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[n.Name]; exists {
		return
	}

	node := &Node{Name: n.Name, Addr: n.Addr}
	r.nodes[n.Name] = node

	for i := 0; i < r.vnodesN; i++ {
		pos := hash32(vnodeKey(n.Name, i))
		r.positions = append(r.positions, pos)
		r.nodeMap[pos] = node
	}

	sort.Slice(r.positions, func(i, j int) bool {
		return r.positions[i] < r.positions[j]
	})
}

// RemoveNode removes all vNode positions for the named physical node.
func (r *VNodeRing) RemoveNode(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.nodes[name]; !exists {
		return
	}

	delete(r.nodes, name)

	// Collect positions to remove
	toRemove := make(map[uint32]bool, r.vnodesN)
	for i := 0; i < r.vnodesN; i++ {
		pos := hash32(vnodeKey(name, i))
		toRemove[pos] = true
		delete(r.nodeMap, pos)
	}

	// Rebuild the sorted slice without the removed positions
	filtered := r.positions[:0]
	for _, p := range r.positions {
		if !toRemove[p] {
			filtered = append(filtered, p)
		}
	}
	r.positions = filtered
}

// GetNode returns the responsible node for a key using the vNode ring.
func (r *VNodeRing) GetNode(key string) *Node {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.positions) == 0 {
		return nil
	}

	h := hash32(key)
	idx := sort.Search(len(r.positions), func(i int) bool {
		return r.positions[i] >= h
	})
	if idx == len(r.positions) {
		idx = 0
	}
	return r.nodeMap[r.positions[idx]]
}

// NodeCount returns the number of physical nodes.
func (r *VNodeRing) NodeCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.nodes)
}

// Nodes returns a snapshot of all physical nodes.
func (r *VNodeRing) Nodes() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		result = append(result, *n)
	}
	return result
}

// Distribution returns per-node key counts and statistical uniformity metrics
// for the provided set of keys.
func (r *VNodeRing) Distribution(keys []string) DistributionStats {
	counts := make(map[string]int)
	for _, k := range keys {
		n := r.GetNode(k)
		if n != nil {
			counts[n.Name]++
		}
	}

	var total int
	min, max := math.MaxInt64, 0
	for _, c := range counts {
		total += c
		if c < min {
			min = c
		}
		if c > max {
			max = c
		}
	}
	if min == math.MaxInt64 {
		min = 0
	}

	n := len(counts)
	mean := 0.0
	if n > 0 {
		mean = float64(total) / float64(n)
	}

	// Coefficient of variation (std dev / mean) expressed as a fraction
	var variance float64
	for _, c := range counts {
		diff := float64(c) - mean
		variance += diff * diff
	}
	stdDev := 0.0
	if n > 1 {
		stdDev = math.Sqrt(variance/float64(n)) / mean
	}

	return DistributionStats{
		KeysPerNode: counts,
		StdDev:      stdDev,
		Min:         min,
		Max:         max,
	}
}

// ── v2 — Dynamic membership with remapping stats ─────────────────────────────
//
// Lesson: the 1/N remapping guarantee is a theoretical bound.
// Adding 1 node to a 5-node ring should remap exactly 1/6 of all keys (~16.7%).
// In practice with vNodes it converges to that bound quickly.
//
// ManagedRing wraps VNodeRing and exposes AddNodeTracked / RemoveNodeTracked
// which accept a sample of existing key assignments, compute the remapping
// that actually happened, and return RemapStats.

// ManagedRing is the v2 ring that measures key remapping on membership changes.
type ManagedRing struct {
	*VNodeRing
}

// NewManaged creates a v2 ring with the given virtual node count.
func NewManaged(vnodesN int) *ManagedRing {
	return &ManagedRing{NewVNode(vnodesN)}
}

// AddNodeTracked adds a node and measures how many of the sample keys moved.
//
// Algorithm:
//  1. Record the current owner for each sample key.
//  2. Add the node (which steals the predecessor range for each of its vNode positions).
//  3. Re-check owners and count keys that changed.
func (m *ManagedRing) AddNodeTracked(n Node, sampleKeys []string) RemapStats {
	oldOwners := m.snapshotOwners(sampleKeys)
	oldCount := m.NodeCount()

	m.AddNode(n)

	newOwners := m.snapshotOwners(sampleKeys)
	remapped := countChanged(oldOwners, newOwners)

	theoretical := 0.0
	if oldCount > 0 {
		theoretical = 1.0 / float64(oldCount+1)
	}

	return RemapStats{
		TotalKeys:    len(sampleKeys),
		RemappedKeys: remapped,
		RemapRate:    float64(remapped) / float64(len(sampleKeys)),
		Theoretical:  theoretical,
	}
}

// RemoveNodeTracked removes a node and measures key migration.
func (m *ManagedRing) RemoveNodeTracked(name string, sampleKeys []string) RemapStats {
	oldOwners := m.snapshotOwners(sampleKeys)
	oldCount := m.NodeCount()

	m.RemoveNode(name)

	newOwners := m.snapshotOwners(sampleKeys)
	remapped := countChanged(oldOwners, newOwners)

	theoretical := 0.0
	if oldCount > 0 {
		theoretical = 1.0 / float64(oldCount)
	}

	return RemapStats{
		TotalKeys:    len(sampleKeys),
		RemappedKeys: remapped,
		RemapRate:    float64(remapped) / float64(len(sampleKeys)),
		Theoretical:  theoretical,
	}
}

// snapshotOwners returns a map of key → node name for all sample keys.
func (m *ManagedRing) snapshotOwners(keys []string) map[string]string {
	owners := make(map[string]string, len(keys))
	for _, k := range keys {
		n := m.GetNode(k)
		if n != nil {
			owners[k] = n.Name
		}
	}
	return owners
}

// countChanged returns how many keys have a different owner in the two snapshots.
func countChanged(before, after map[string]string) int {
	count := 0
	for k, oldOwner := range before {
		if after[k] != oldOwner {
			count++
		}
	}
	return count
}
