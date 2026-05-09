// Package swim implements the SWIM gossip protocol in three progressive stages.
//
// Three versions live in this file:
//
//	v0 — Simple gossip dissemination. Every 500ms each node picks 3 random peers
//	     and sends its full membership list over UDP. On receive, merge: add any
//	     unknown nodes, update timestamps.
//	     Key lesson: information spreads in O(log N) rounds. With fanout F=3,
//	     after k rounds you've reached up to F^k nodes. 10 rounds covers 59,049.
//
//	v1 — SWIM failure detection. Every 1s each node Pings a random peer.
//	     If no Ack within 200ms, send PingReq to 2 other nodes asking them
//	     to probe indirectly. If still no Ack, mark Suspect. After 3 failed probe
//	     cycles, mark Dead and gossip the Dead status.
//	     Key lesson: indirect probe cuts false positives from ~10% to ~2%.
//
//	v2 — Piggybacking. Instead of sending full membership state, send only the
//	     last K=8 recent changes (Alive/Suspect/Dead + incarnation number).
//	     Events ride on every Ping/Ack — no separate gossip message needed.
//	     Events decay: stop propagating after log(N)+2 transmissions.
//	     Key lesson: piggybacking halves network traffic by eliminating the
//	     separate gossip round entirely.
package swim

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

// ── Shared constants and types ────────────────────────────────────────────────

const (
	// GossipFanout is the number of random peers to gossip to each round.
	GossipFanout = 3
	// GossipInterval is how often a node initiates a gossip round.
	GossipInterval = 500 * time.Millisecond
	// ProbeInterval is how often SWIM sends a Ping to a random peer.
	ProbeInterval = 1 * time.Second
	// ProbeTimeout is how long to wait for an Ack before indirect probing.
	ProbeTimeout = 200 * time.Millisecond
	// SuspectTimeout is how many failed probe cycles before declaring Dead.
	SuspectThreshold = 3
	// MaxPiggybackEvents is the K in "last K events" for piggybacking (v2).
	MaxPiggybackEvents = 8
)

// NodeStatus represents the membership status of a cluster node.
type NodeStatus int

const (
	StatusAlive   NodeStatus = iota // Node is reachable and healthy.
	StatusSuspect                   // Node failed a probe cycle — not yet declared dead.
	StatusDead                      // Node has been confirmed dead by indirect probing.
)

func (s NodeStatus) String() string {
	switch s {
	case StatusAlive:
		return "alive"
	case StatusSuspect:
		return "suspect"
	case StatusDead:
		return "dead"
	default:
		return "unknown"
	}
}

// Member represents one cluster node in the membership list.
type Member struct {
	Addr        string     `json:"addr"`
	Status      NodeStatus `json:"status"`
	Incarnation uint64     `json:"incarnation"` // bumped when a node refutes a Suspect claim
	LastSeen    time.Time  `json:"lastSeen"`
	// failedProbes tracks consecutive probe failures (used in v1).
	failedProbes int
}

// Message is the UDP message envelope shared by all protocol versions.
type Message struct {
	Type    string            `json:"type"`    // "gossip", "ping", "pingreq", "ack"
	From    string            `json:"from"`    // sender's addr
	Members map[string]Member `json:"members"` // full membership (v0/v1)
	Target  string            `json:"target"`  // for pingreq: the node to probe
	Events  []Event           `json:"events"`  // piggybacked events (v2)
	SeqNo   uint64            `json:"seqNo"`   // probe sequence number for matching Ack
}

// Event is a single membership change, piggybacked on Ping/Ack messages (v2).
type Event struct {
	Addr        string     `json:"addr"`
	Status      NodeStatus `json:"status"`
	Incarnation uint64     `json:"incarnation"`
	// transmitCount tracks how many times this event has been piggybacked.
	// Events are discarded after log(N)+2 transmissions.
	TransmitCount int `json:"transmitCount"`
}

// ── v0 — Simple gossip dissemination ─────────────────────────────────────────
//
// Each node maintains a membership list. Every GossipInterval it picks
// GossipFanout random peers and sends them its full membership list over UDP.
// On receive: merge. New nodes are added; timestamps are updated if the
// incoming entry is fresher.
//
// Analysis: with fanout F=3 and N nodes, each round roughly triples the number
// of infected nodes. After k rounds: min(F^k, N) nodes are infected.
// To reach all N nodes: k = ceil(log_F(N)) rounds.
// For N=1000: log_3(1000) ≈ 6.3 → 7 rounds = 3.5 seconds at 500ms interval.

// NewMember creates a Member value for use in tests and benchmarks.
// The unexported failedProbes field is initialized to zero.
func NewMember(addr string, status NodeStatus, incarnation uint64) Member {
	return Member{
		Addr:        addr,
		Status:      status,
		Incarnation: incarnation,
		LastSeen:    time.Now(),
	}
}

// GossipNode is the v0 gossip node: full-state dissemination only.
type GossipNode struct {
	addr    string
	conn    *net.UDPConn
	members map[string]*Member
	mu      sync.RWMutex
	stopCh  chan struct{}

	// Stats
	roundCount   int64
	messagesSent int64
}

// NewGossipNode creates a v0 gossip node listening on the given UDP address.
// Call Start() to begin the gossip loop.
func NewGossipNode(addr string) (*GossipNode, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve addr %s: %w", addr, err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP %s: %w", addr, err)
	}

	n := &GossipNode{
		addr:    addr,
		conn:    conn,
		members: make(map[string]*Member),
		stopCh:  make(chan struct{}),
	}

	// Register self as Alive.
	n.members[addr] = &Member{
		Addr:     addr,
		Status:   StatusAlive,
		LastSeen: time.Now(),
	}

	return n, nil
}

// Join registers a seed peer. Call before Start() to bootstrap.
func (n *GossipNode) Join(peerAddr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, exists := n.members[peerAddr]; !exists {
		n.members[peerAddr] = &Member{
			Addr:     peerAddr,
			Status:   StatusAlive,
			LastSeen: time.Now(),
		}
	}
}

// Start begins the gossip loop and the UDP receive loop in background goroutines.
func (n *GossipNode) Start() {
	go n.receiveLoop()
	go n.gossipLoop()
}

// Stop gracefully shuts down the gossip node.
func (n *GossipNode) Stop() {
	close(n.stopCh)
	n.conn.Close()
}

// MergeMembers is an exported wrapper around mergeMembers for benchmarking.
func (n *GossipNode) MergeMembers(remote map[string]Member) {
	n.mergeMembers(remote)
}

// Members returns a snapshot of the current membership list.
func (n *GossipNode) Members() map[string]Member {
	n.mu.RLock()
	defer n.mu.RUnlock()
	out := make(map[string]Member, len(n.members))
	for k, v := range n.members {
		out[k] = *v
	}
	return out
}

// Stats returns gossip round and message counters.
func (n *GossipNode) Stats() (rounds int64, messages int64) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.roundCount, n.messagesSent
}

// gossipLoop runs the periodic gossip dissemination (v0).
func (n *GossipNode) gossipLoop() {
	ticker := time.NewTicker(GossipInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.doGossipRound()
		case <-n.stopCh:
			return
		}
	}
}

// doGossipRound picks GossipFanout random peers and sends them the full membership.
func (n *GossipNode) doGossipRound() {
	n.mu.Lock()
	peers := n.pickRandomPeers(GossipFanout)
	snapshot := n.memberSnapshot()
	n.roundCount++
	n.mu.Unlock()

	msg := Message{
		Type:    "gossip",
		From:    n.addr,
		Members: snapshot,
	}

	for _, peer := range peers {
		if err := n.sendTo(peer, msg); err != nil {
			log.Printf("gossip send to %s failed: %v", peer, err)
		} else {
			n.mu.Lock()
			n.messagesSent++
			n.mu.Unlock()
		}
	}
}

// receiveLoop reads UDP packets and dispatches them.
func (n *GossipNode) receiveLoop() {
	buf := make([]byte, 65535)
	for {
		nRead, _, err := n.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-n.stopCh:
				return
			default:
				log.Printf("UDP read error on %s: %v", n.addr, err)
				continue
			}
		}
		var msg Message
		if err := json.Unmarshal(buf[:nRead], &msg); err != nil {
			log.Printf("unmarshal error from %s: %v", n.addr, err)
			continue
		}
		n.handleMessage(msg)
	}
}

// handleMessage routes incoming messages to the appropriate handler.
func (n *GossipNode) handleMessage(msg Message) {
	switch msg.Type {
	case "gossip":
		n.mergeMembers(msg.Members)
	}
}

// mergeMembers integrates remote membership state with the local view.
// Rule: if the remote entry has a higher incarnation, it wins.
// If same incarnation, prefer Alive > Suspect > Dead (higher status = later info).
func (n *GossipNode) mergeMembers(remote map[string]Member) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for addr, rm := range remote {
		local, exists := n.members[addr]
		if !exists {
			// Unknown node — add it.
			m := rm
			n.members[addr] = &m
			continue
		}
		// Higher incarnation always wins.
		if rm.Incarnation > local.Incarnation {
			m := rm
			n.members[addr] = &m
			continue
		}
		// Same incarnation: later status update wins.
		// Alive (0) < Suspect (1) < Dead (2): higher value = worse state.
		if rm.Incarnation == local.Incarnation && rm.Status > local.Status {
			local.Status = rm.Status
			local.LastSeen = rm.LastSeen
		}
	}
}

// pickRandomPeers returns up to n random peer addresses (not self).
// Caller must hold mu.Lock().
func (n *GossipNode) pickRandomPeers(count int) []string {
	peers := make([]string, 0, len(n.members)-1)
	for addr := range n.members {
		if addr != n.addr {
			peers = append(peers, addr)
		}
	}
	rand.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	if count > len(peers) {
		count = len(peers)
	}
	return peers[:count]
}

// memberSnapshot returns a shallow copy of the membership map for sending.
// Caller must hold mu (at least RLock).
func (n *GossipNode) memberSnapshot() map[string]Member {
	snap := make(map[string]Member, len(n.members))
	for k, v := range n.members {
		snap[k] = *v
	}
	return snap
}

// sendTo marshals a message and sends it to the given UDP address.
func (n *GossipNode) sendTo(addr string, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", addr, err)
	}
	_, err = n.conn.WriteTo(data, udpAddr)
	return err
}

// ── v1 — SWIM failure detection ───────────────────────────────────────────────
//
// Adds to v0: a probe loop that runs every ProbeInterval.
//
//   probe cycle:
//     1. Pick a random alive peer as target.
//     2. Send Ping. Wait ProbeTimeout for Ack.
//     3. If no Ack: send PingReq to 2 other random nodes, asking them to Ping target.
//        Wait another ProbeTimeout for forwarded Ack.
//     4. If still no Ack: increment target.failedProbes.
//        If failedProbes >= SuspectThreshold: mark Dead and gossip.
//        Else: mark Suspect and gossip.
//
// False positive reduction: a transient network hiccup between A and B shows
// up as a missed direct Ping. But C and D can still reach B — so their
// PingReq succeeds, and A does NOT mark B as Suspect. The false positive rate
// drops from ~packet_loss_rate to ~packet_loss_rate^3 (all three must fail).

// SWIMNode extends GossipNode with SWIM failure detection.
type SWIMNode struct {
	*GossipNode
	seqNo  uint64            // monotonic probe sequence number
	ackChs map[uint64]chan struct{} // seqNo → channel that gets closed on Ack
	ackMu  sync.Mutex
}

// NewSWIMNode creates a v1 SWIM node.
func NewSWIMNode(addr string) (*SWIMNode, error) {
	g, err := NewGossipNode(addr)
	if err != nil {
		return nil, err
	}
	return &SWIMNode{
		GossipNode: g,
		ackChs:     make(map[uint64]chan struct{}),
	}, nil
}

// Start begins gossip, probe, and receive loops.
func (n *SWIMNode) Start() {
	go n.receiveLoopSWIM()
	go n.gossipLoop()
	go n.probeLoop()
}

// probeLoop runs the SWIM probe cycle every ProbeInterval.
func (n *SWIMNode) probeLoop() {
	ticker := time.NewTicker(ProbeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.doProbeCycle()
		case <-n.stopCh:
			return
		}
	}
}

// doProbeCycle implements one full SWIM probe: Ping → optional PingReq → verdict.
func (n *SWIMNode) doProbeCycle() {
	n.mu.Lock()
	target := n.pickProbeTarget()
	if target == "" {
		n.mu.Unlock()
		return
	}
	seq := n.nextSeq()
	n.mu.Unlock()

	// Step 1: Direct Ping.
	ackCh := n.registerAck(seq)
	ping := Message{
		Type:  "ping",
		From:  n.addr,
		SeqNo: seq,
	}
	if err := n.sendTo(target, ping); err != nil {
		log.Printf("ping send to %s failed: %v", target, err)
	}

	if n.waitAck(ackCh, ProbeTimeout) {
		// Direct Ping succeeded — target is alive.
		n.markAlive(target)
		return
	}
	n.deregisterAck(seq)

	// Step 2: Indirect PingReq via 2 other nodes.
	n.mu.Lock()
	helpers := n.pickRandomPeers(2)
	seq2 := n.nextSeq()
	n.mu.Unlock()

	ackCh2 := n.registerAck(seq2)
	for _, helper := range helpers {
		pr := Message{
			Type:   "pingreq",
			From:   n.addr,
			Target: target,
			SeqNo:  seq2,
		}
		if err := n.sendTo(helper, pr); err != nil {
			log.Printf("pingreq send to %s failed: %v", helper, err)
		}
	}

	if n.waitAck(ackCh2, ProbeTimeout) {
		// Indirect probe succeeded — A's link to B was the problem, not B.
		n.markAlive(target)
		return
	}
	n.deregisterAck(seq2)

	// Step 3: Both direct and indirect failed — increment suspect counter.
	n.mu.Lock()
	if m, ok := n.members[target]; ok {
		m.failedProbes++
		if m.failedProbes >= SuspectThreshold {
			if m.Status != StatusDead {
				m.Status = StatusDead
				log.Printf("[%s] marking %s as DEAD (failed %d probe cycles)",
					n.addr, target, m.failedProbes)
			}
		} else {
			if m.Status == StatusAlive {
				m.Status = StatusSuspect
				log.Printf("[%s] marking %s as SUSPECT (failed probe %d)",
					n.addr, target, m.failedProbes)
			}
		}
	}
	n.mu.Unlock()
}

// receiveLoopSWIM extends the base receive loop to handle Ping/Ack/PingReq.
func (n *SWIMNode) receiveLoopSWIM() {
	buf := make([]byte, 65535)
	for {
		nRead, remoteAddr, err := n.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-n.stopCh:
				return
			default:
				continue
			}
		}
		var msg Message
		if err := json.Unmarshal(buf[:nRead], &msg); err != nil {
			continue
		}
		_ = remoteAddr
		n.handleSWIMMessage(msg)
	}
}

// handleSWIMMessage dispatches gossip/ping/pingreq/ack messages.
func (n *SWIMNode) handleSWIMMessage(msg Message) {
	switch msg.Type {
	case "gossip":
		n.mergeMembers(msg.Members)

	case "ping":
		// Respond with Ack carrying our current membership state.
		ack := Message{
			Type:    "ack",
			From:    n.addr,
			SeqNo:   msg.SeqNo,
			Members: n.Members(),
		}
		if err := n.sendTo(msg.From, ack); err != nil {
			log.Printf("ack send to %s failed: %v", msg.From, err)
		}

	case "pingreq":
		// Probe the target on behalf of the requester.
		seq := n.nextSeqLocked()
		ackCh := n.registerAck(seq)
		ping := Message{
			Type:  "ping",
			From:  n.addr,
			SeqNo: seq,
		}
		if err := n.sendTo(msg.Target, ping); err != nil {
			log.Printf("pingreq forward-ping to %s failed: %v", msg.Target, err)
		}
		// If we get an Ack, forward it back to the original requester.
		go func(requester string, origSeq uint64) {
			if n.waitAck(ackCh, ProbeTimeout) {
				fwd := Message{
					Type:  "ack",
					From:  n.addr,
					SeqNo: origSeq,
				}
				if err := n.sendTo(requester, fwd); err != nil {
					log.Printf("forward ack to %s failed: %v", requester, err)
				}
			}
			n.deregisterAck(seq)
		}(msg.From, msg.SeqNo)

	case "ack":
		n.mergeMembers(msg.Members)
		n.ackMu.Lock()
		if ch, ok := n.ackChs[msg.SeqNo]; ok {
			close(ch)
			delete(n.ackChs, msg.SeqNo)
		}
		n.ackMu.Unlock()
	}
}

// pickProbeTarget selects a random alive (or suspect) peer to probe.
// Caller must hold mu.
func (n *SWIMNode) pickProbeTarget() string {
	candidates := make([]string, 0)
	for addr, m := range n.members {
		if addr != n.addr && m.Status != StatusDead {
			candidates = append(candidates, addr)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[rand.Intn(len(candidates))]
}

// markAlive resets a node's probe failure count and status.
func (n *SWIMNode) markAlive(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if m, ok := n.members[addr]; ok {
		m.Status = StatusAlive
		m.failedProbes = 0
		m.LastSeen = time.Now()
	}
}

// nextSeq increments and returns the probe sequence number.
// Caller must hold mu.
func (n *SWIMNode) nextSeq() uint64 {
	n.seqNo++
	return n.seqNo
}

// nextSeqLocked is nextSeq without requiring the caller to hold mu.
func (n *SWIMNode) nextSeqLocked() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.nextSeq()
}

// registerAck creates a channel that will be closed when the given seq is Acked.
func (n *SWIMNode) registerAck(seq uint64) chan struct{} {
	ch := make(chan struct{})
	n.ackMu.Lock()
	n.ackChs[seq] = ch
	n.ackMu.Unlock()
	return ch
}

// deregisterAck removes a pending Ack channel (cleanup on timeout).
func (n *SWIMNode) deregisterAck(seq uint64) {
	n.ackMu.Lock()
	delete(n.ackChs, seq)
	n.ackMu.Unlock()
}

// waitAck blocks until the channel is closed (Ack received) or timeout.
func (n *SWIMNode) waitAck(ch chan struct{}, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

// ── v2 — Piggybacking and infection-style dissemination ──────────────────────
//
// Instead of sending the full membership map in every gossip message, v2 sends
// only the most recent K=8 events. Each event records a node status change
// (Alive/Suspect/Dead) with an incarnation number.
//
// Events piggyback on Ping and Ack messages — the health check and the gossip
// message become a single UDP packet. This halves traffic for large clusters.
//
// Incarnation numbers: when node B hears a Suspect rumor about itself, it
// increments its own incarnation and broadcasts Alive{incarnation: N+1}.
// This overrides the Suspect(incarnation: N) — refutation wins.
//
// Event decay: each event is transmitted at most log2(N)+2 times, then
// discarded from the piggyback queue. This bounds the per-message overhead
// at O(K) regardless of cluster size.

// PiggybackNode is the v2 SWIM node with piggybacking.
type PiggybackNode struct {
	*SWIMNode
	eventQueue []Event // recent K events to piggyback
	eventMu    sync.Mutex
	nodeCount  int // approximate N for decay calculation
}

// NewPiggybackNode creates a v2 node with event piggybacking.
func NewPiggybackNode(addr string) (*PiggybackNode, error) {
	s, err := NewSWIMNode(addr)
	if err != nil {
		return nil, err
	}
	return &PiggybackNode{
		SWIMNode:   s,
		eventQueue: make([]Event, 0, MaxPiggybackEvents),
		nodeCount:  1,
	}, nil
}

// Start begins all loops.
func (n *PiggybackNode) Start() {
	go n.receiveLoopV2()
	go n.gossipLoopV2()
	go n.probeLoopV2()
}

// pushEvent enqueues a membership change event.
// Events beyond MaxPiggybackEvents evict the oldest.
func (n *PiggybackNode) pushEvent(addr string, status NodeStatus, incarnation uint64) {
	n.eventMu.Lock()
	defer n.eventMu.Unlock()

	// De-duplicate: if the same addr already has a pending event, overwrite it.
	for i, e := range n.eventQueue {
		if e.Addr == addr {
			n.eventQueue[i] = Event{
				Addr:        addr,
				Status:      status,
				Incarnation: incarnation,
			}
			return
		}
	}

	ev := Event{Addr: addr, Status: status, Incarnation: incarnation}
	if len(n.eventQueue) >= MaxPiggybackEvents {
		// Evict oldest (index 0).
		n.eventQueue = append(n.eventQueue[1:], ev)
	} else {
		n.eventQueue = append(n.eventQueue, ev)
	}
}

// pickEvents returns up to MaxPiggybackEvents events, incrementing their
// transmit counts and removing those that have exceeded the decay limit.
func (n *PiggybackNode) pickEvents() []Event {
	n.eventMu.Lock()
	defer n.eventMu.Unlock()

	n.mu.RLock()
	memberCount := len(n.members)
	n.mu.RUnlock()

	maxTransmit := int(math.Log2(float64(memberCount+1))) + 2
	if maxTransmit < 2 {
		maxTransmit = 2
	}

	selected := make([]Event, 0, MaxPiggybackEvents)
	remaining := make([]Event, 0, len(n.eventQueue))
	for _, e := range n.eventQueue {
		selected = append(selected, e)
		e.TransmitCount++
		if e.TransmitCount < maxTransmit {
			remaining = append(remaining, e)
		}
	}
	n.eventQueue = remaining
	return selected
}

// PickEvents is an exported wrapper around pickEvents for benchmarking.
func (n *PiggybackNode) PickEvents() []Event {
	return n.pickEvents()
}

// InjectDead directly marks an address as Dead in the local membership.
// This is a test helper that simulates receiving a Dead gossip event without
// waiting for the full probe cycle. Not for production use.
func (n *PiggybackNode) InjectDead(addr string) {
	n.mu.Lock()
	if m, ok := n.members[addr]; ok {
		m.Status = StatusDead
	}
	n.mu.Unlock()
	n.pushEvent(addr, StatusDead, 0)
}

// mergeEvents integrates piggybacked events into the local membership view.
func (n *PiggybackNode) mergeEvents(events []Event) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, ev := range events {
		local, exists := n.members[ev.Addr]
		if !exists {
			n.members[ev.Addr] = &Member{
				Addr:        ev.Addr,
				Status:      ev.Status,
				Incarnation: ev.Incarnation,
				LastSeen:    time.Now(),
			}
			// Queue event for further propagation.
			go n.pushEvent(ev.Addr, ev.Status, ev.Incarnation)
			continue
		}
		if ev.Incarnation > local.Incarnation ||
			(ev.Incarnation == local.Incarnation && ev.Status > local.Status) {
			// Self-refutation: if we hear we are Suspect, bump incarnation and announce Alive.
			if ev.Addr == n.addr && ev.Status == StatusSuspect {
				local.Incarnation++
				local.Status = StatusAlive
				go n.pushEvent(n.addr, StatusAlive, local.Incarnation)
				continue
			}
			local.Status = ev.Status
			local.Incarnation = ev.Incarnation
			local.LastSeen = time.Now()
			go n.pushEvent(ev.Addr, ev.Status, ev.Incarnation)
		}
	}
}

// gossipLoopV2 sends piggybacked events instead of full state.
func (n *PiggybackNode) gossipLoopV2() {
	ticker := time.NewTicker(GossipInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.doGossipRoundV2()
		case <-n.stopCh:
			return
		}
	}
}

// doGossipRoundV2 gossips only recent events (not full state).
func (n *PiggybackNode) doGossipRoundV2() {
	n.mu.Lock()
	peers := n.pickRandomPeers(GossipFanout)
	n.roundCount++
	n.mu.Unlock()

	events := n.pickEvents()
	msg := Message{
		Type:   "gossip",
		From:   n.addr,
		Events: events,
	}
	for _, peer := range peers {
		if err := n.sendTo(peer, msg); err != nil {
			log.Printf("v2 gossip send to %s failed: %v", peer, err)
		} else {
			n.mu.Lock()
			n.messagesSent++
			n.mu.Unlock()
		}
	}
}

// probeLoopV2 runs the SWIM probe cycle with piggybacked events.
func (n *PiggybackNode) probeLoopV2() {
	ticker := time.NewTicker(ProbeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			n.doProbeCycleV2()
		case <-n.stopCh:
			return
		}
	}
}

// doProbeCycleV2 probes with piggybacked events on every Ping/Ack.
func (n *PiggybackNode) doProbeCycleV2() {
	n.mu.Lock()
	target := n.pickProbeTarget()
	if target == "" {
		n.mu.Unlock()
		return
	}
	seq := n.nextSeq()
	n.mu.Unlock()

	events := n.pickEvents()

	ackCh := n.registerAck(seq)
	ping := Message{
		Type:   "ping",
		From:   n.addr,
		SeqNo:  seq,
		Events: events,
	}
	if err := n.sendTo(target, ping); err != nil {
		log.Printf("v2 ping to %s failed: %v", target, err)
	}

	if n.waitAck(ackCh, ProbeTimeout) {
		n.markAlive(target)
		return
	}
	n.deregisterAck(seq)

	// Indirect probe via helpers.
	n.mu.Lock()
	helpers := n.pickRandomPeers(2)
	seq2 := n.nextSeq()
	n.mu.Unlock()

	ackCh2 := n.registerAck(seq2)
	for _, helper := range helpers {
		pr := Message{
			Type:   "pingreq",
			From:   n.addr,
			Target: target,
			SeqNo:  seq2,
			Events: n.pickEvents(),
		}
		if err := n.sendTo(helper, pr); err != nil {
			log.Printf("v2 pingreq to %s failed: %v", helper, err)
		}
	}

	if n.waitAck(ackCh2, ProbeTimeout) {
		n.markAlive(target)
		return
	}
	n.deregisterAck(seq2)

	n.mu.Lock()
	if m, ok := n.members[target]; ok {
		m.failedProbes++
		if m.failedProbes >= SuspectThreshold {
			if m.Status != StatusDead {
				m.Status = StatusDead
				go n.pushEvent(target, StatusDead, m.Incarnation)
				log.Printf("[%s] v2: marking %s as DEAD", n.addr, target)
			}
		} else if m.Status == StatusAlive {
			m.Status = StatusSuspect
			go n.pushEvent(target, StatusSuspect, m.Incarnation)
			log.Printf("[%s] v2: marking %s as SUSPECT", n.addr, target)
		}
	}
	n.mu.Unlock()
}

// receiveLoopV2 handles all message types for v2.
func (n *PiggybackNode) receiveLoopV2() {
	buf := make([]byte, 65535)
	for {
		nRead, _, err := n.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-n.stopCh:
				return
			default:
				continue
			}
		}
		var msg Message
		if err := json.Unmarshal(buf[:nRead], &msg); err != nil {
			continue
		}
		n.handleV2Message(msg)
	}
}

// handleV2Message dispatches v2 protocol messages.
func (n *PiggybackNode) handleV2Message(msg Message) {
	// Always merge piggybacked events.
	if len(msg.Events) > 0 {
		n.mergeEvents(msg.Events)
	}
	// Also merge full membership if present (backward compat with v0/v1 gossip).
	if msg.Members != nil {
		n.mergeMembers(msg.Members)
	}

	switch msg.Type {
	case "ping":
		ack := Message{
			Type:   "ack",
			From:   n.addr,
			SeqNo:  msg.SeqNo,
			Events: n.pickEvents(),
		}
		if err := n.sendTo(msg.From, ack); err != nil {
			log.Printf("v2 ack send to %s failed: %v", msg.From, err)
		}

	case "pingreq":
		seq := n.nextSeqLocked()
		ackCh := n.registerAck(seq)
		ping := Message{
			Type:   "ping",
			From:   n.addr,
			SeqNo:  seq,
			Events: n.pickEvents(),
		}
		if err := n.sendTo(msg.Target, ping); err != nil {
			log.Printf("v2 pingreq forward ping to %s failed: %v", msg.Target, err)
		}
		go func(requester string, origSeq uint64) {
			if n.waitAck(ackCh, ProbeTimeout) {
				fwd := Message{
					Type:   "ack",
					From:   n.addr,
					SeqNo:  origSeq,
					Events: n.pickEvents(),
				}
				if err := n.sendTo(requester, fwd); err != nil {
					log.Printf("v2 forward ack to %s failed: %v", requester, err)
				}
			}
			n.deregisterAck(seq)
		}(msg.From, msg.SeqNo)

	case "ack":
		n.ackMu.Lock()
		if ch, ok := n.ackChs[msg.SeqNo]; ok {
			close(ch)
			delete(n.ackChs, msg.SeqNo)
		}
		n.ackMu.Unlock()
	}
}
