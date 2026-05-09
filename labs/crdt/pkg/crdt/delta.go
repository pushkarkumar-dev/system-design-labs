package crdt

// DeltaGCounter is a GCounter with delta-state protocol support.
//
// Instead of always shipping the full state (all N entries) to peers, a node
// computes a "delta" containing only the entries that changed since the peer's
// last known causal context. This dramatically reduces bandwidth:
//
//   - Full state: O(N) entries where N is the number of nodes
//   - Delta: O(1) entries for a single increment on one node
//
// The delta is itself a valid GCounter — it can be merged into any replica
// using the same max-per-entry merge rule. This makes delta application
// idempotent: merging the same delta twice produces the same result.
//
// The bandwidth improvement:
//
//	100-node cluster, 1 increment/node/sec, 10 gossip partners:
//	  Full state: 100 entries × 10 partners = 1,000 entries/sec
//	  Delta:        1 entry  × 10 partners =    10 entries/sec  (100x reduction)
type DeltaGCounter struct {
	state   GCounter       // full state (all node entries)
	context CausalContext  // causal context: what we've applied
	seqno   uint64         // this node's current sequence number
	nodeID  string         // this node's ID
}

// NewDeltaGCounter creates a new DeltaGCounter for the given node.
func NewDeltaGCounter(nodeID string) DeltaGCounter {
	return DeltaGCounter{
		state:   NewGCounter(),
		context: NewCausalContext(),
		nodeID:  nodeID,
	}
}

// Increment increments the counter for this node and advances the causal context.
// Returns the delta (a GCounter containing only this node's updated entry).
func (d *DeltaGCounter) Increment() GCounter {
	d.seqno++
	d.state.Increment(d.nodeID)
	d.context.Observe(d.nodeID, d.seqno)

	// The delta is just this node's current value — a single-entry GCounter.
	delta := NewGCounter()
	delta.counts[d.nodeID] = d.state.counts[d.nodeID]
	return delta
}

// Value returns the current counter value (sum of all entries).
func (d *DeltaGCounter) Value() int64 {
	return d.state.Value()
}

// ApplyDelta merges a delta GCounter into this counter's state.
// The delta is typically a small subset of the full state.
// Application is idempotent: max-per-entry ensures applying twice is safe.
func (d *DeltaGCounter) ApplyDelta(delta GCounter) {
	d.state.Merge(delta)
	// Update causal context from the delta's entries.
	for nodeID, count := range delta.counts {
		// Use count as the sequence number proxy (works for GCounters).
		if uint64(count) > d.context.SeenFrom(nodeID) {
			d.context.Observe(nodeID, uint64(count))
		}
	}
}

// GenerateDelta returns the entries in this counter that are newer than
// what the peer has seen (as described by peerContext).
// If peerContext is empty, returns the full state.
func (d *DeltaGCounter) GenerateDelta(peerContext CausalContext) GCounter {
	delta := NewGCounter()
	for nodeID, count := range d.state.counts {
		// Include this entry if the peer hasn't seen our latest value for nodeID.
		peerSeen := peerContext.SeenFrom(nodeID)
		if uint64(count) > peerSeen {
			delta.counts[nodeID] = count
		}
	}
	return delta
}

// FullState returns the complete GCounter state.
func (d *DeltaGCounter) FullState() GCounter {
	return d.state.Clone()
}

// Context returns this node's causal context (what it has applied so far).
func (d *DeltaGCounter) Context() CausalContext {
	return d.context.Clone()
}

// NodeCount returns the number of nodes tracked in this counter's state.
func (d *DeltaGCounter) NodeCount() int {
	return len(d.state.counts)
}

// DeltaSize returns the number of entries in a delta compared to the full state size.
// Used for testing the bandwidth reduction claim.
func DeltaSize(delta, fullState GCounter) (deltaEntries, fullEntries int) {
	return len(delta.counts), len(fullState.counts)
}

// CrdtNode orchestrates multiple CRDTs on a single node in a distributed system.
//
// It maintains a DeltaGCounter and handles the delta-sync protocol:
// generating deltas for peers and applying received deltas.
// In a real system, CrdtNode would manage multiple CRDT types and maintain
// per-peer causal contexts to know which deltas each peer needs.
type CrdtNode struct {
	nodeID  string
	counter DeltaGCounter
	// peerContexts tracks what each peer has already seen.
	// In a real system this would be persisted and synced via anti-entropy.
	peerContexts map[string]CausalContext
}

// NewCrdtNode creates a new node with the given ID.
func NewCrdtNode(nodeID string) *CrdtNode {
	return &CrdtNode{
		nodeID:       nodeID,
		counter:      NewDeltaGCounter(nodeID),
		peerContexts: make(map[string]CausalContext),
	}
}

// Increment increments this node's counter and returns the delta to send to peers.
func (n *CrdtNode) Increment() GCounter {
	return n.counter.Increment()
}

// ApplyDelta applies a received delta from another node.
func (n *CrdtNode) ApplyDelta(delta GCounter) {
	n.counter.ApplyDelta(delta)
}

// GenerateDeltaFor returns the delta to send to peerID.
// Only includes entries the peer hasn't seen yet.
func (n *CrdtNode) GenerateDeltaFor(peerID string) GCounter {
	peerCtx := n.peerContexts[peerID]
	return n.counter.GenerateDelta(peerCtx)
}

// AcknowledgeDelta records that peerID has successfully applied up to the
// entries described by the given delta (advances the peer's known context).
func (n *CrdtNode) AcknowledgeDelta(peerID string, delta GCounter) {
	ctx, ok := n.peerContexts[peerID]
	if !ok {
		ctx = NewCausalContext()
	}
	for nodeID, count := range delta.counts {
		ctx.Observe(nodeID, uint64(count))
	}
	n.peerContexts[peerID] = ctx
}

// Value returns the current counter value at this node.
func (n *CrdtNode) Value() int64 {
	return n.counter.Value()
}

// PeerContexts returns a copy of all tracked peer contexts (for debugging).
func (n *CrdtNode) PeerContexts() map[string]CausalContext {
	result := make(map[string]CausalContext, len(n.peerContexts))
	for k, v := range n.peerContexts {
		result[k] = v.Clone()
	}
	return result
}

// FullState returns this node's full counter state.
func (n *CrdtNode) FullState() GCounter {
	return n.counter.FullState()
}
