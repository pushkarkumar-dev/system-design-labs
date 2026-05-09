// Package raft implements the Raft consensus algorithm in three progressive stages.
//
// v0 — Leader election only (~150 LoC)
//
//	Three goroutines simulate three Raft nodes communicating over Go channels.
//	Each node has: state (Follower/Candidate/Leader), currentTerm, votedFor,
//	and a randomized election timeout (150–300 ms).  The leader sends periodic
//	heartbeats (empty AppendEntries).  When a follower's timer fires it becomes
//	a Candidate, increments its term, and requests votes.  A majority wins.
//	Key lesson: randomized timeouts prevent livelock; if two candidates tie the
//	next term resolves it.
//
// v1 — Log replication (~350 LoC total)
//
//	Add []LogEntry (term + command), commitIndex, lastApplied, nextIndex[],
//	matchIndex[] per leader.  AppendEntries carries real entries plus the
//	prevLogIndex/prevLogTerm consistency check.  The leader advances commitIndex
//	once a majority of matchIndex[] values reach it.  Applied entries update a
//	simple map[string]string state machine.
//	Key lesson: the log is the source of truth; replay it to rebuild any state
//	machine.
//
// v2 — Snapshots (~200 LoC added)
//
//	When the log exceeds 1 000 entries the leader takes a snapshot (JSON of the
//	state machine) and truncates the log.  InstallSnapshot RPC brings a lagging
//	follower current in one round-trip.
//	Key lesson: logs grow unboundedly without compaction; snapshots are the only
//	escape hatch.
package raft

import (
	"encoding/json"
	"math/rand"
	"sync"
	"time"
)

// ── Shared constants and types ────────────────────────────────────────────────

const (
	electionTimeoutMin = 150 * time.Millisecond
	electionTimeoutMax = 300 * time.Millisecond
	heartbeatInterval  = 50 * time.Millisecond

	snapshotThreshold = 1000 // compact when log grows past this many entries
)

// State is the role of a Raft node.
type State int

const (
	Follower State = iota
	Candidate
	Leader
)

func (s State) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// LogEntry is one entry in the replicated log.
type LogEntry struct {
	Term    int    `json:"term"`
	Command string `json:"command"`
}

// ── RPC argument and reply types ─────────────────────────────────────────────

// RequestVoteArgs is sent by a Candidate to solicit a vote.
type RequestVoteArgs struct {
	Term         int
	CandidateID  int
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply is returned by the voter.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// AppendEntriesArgs is sent by the Leader for heartbeats and log replication.
type AppendEntriesArgs struct {
	Term         int
	LeaderID     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

// AppendEntriesReply is returned by the follower.
type AppendEntriesReply struct {
	Term    int
	Success bool
	// ConflictIndex/ConflictTerm allow fast log roll-back (not implemented but
	// included so callers can inspect them).
	ConflictIndex int
	ConflictTerm  int
}

// InstallSnapshotArgs is sent by the Leader to bring a lagging follower current.
type InstallSnapshotArgs struct {
	Term              int
	LeaderID          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte // JSON-encoded state machine snapshot
}

// InstallSnapshotReply is returned after snapshot installation.
type InstallSnapshotReply struct {
	Term int
}

// ── Snapshot ──────────────────────────────────────────────────────────────────

// Snapshot holds the compacted state machine and the log metadata at the point
// of compaction.
type Snapshot struct {
	LastIncludedIndex int               `json:"lastIncludedIndex"`
	LastIncludedTerm  int               `json:"lastIncludedTerm"`
	StateMachine      map[string]string `json:"stateMachine"`
}

// ── Node ─────────────────────────────────────────────────────────────────────

// Node is a single Raft participant.
//
// All exported fields are read-only after Start(); use the accessor methods for
// concurrent access.
type Node struct {
	mu sync.Mutex

	// Identity
	id    int
	peers []*Node // direct references; in real Raft these would be network RPCs

	// Persistent state (would be written to stable storage in production)
	currentTerm int
	votedFor    int // -1 if not voted in this term
	log         []LogEntry

	// Snapshot state (v2)
	snapshot          *Snapshot
	snapshotLastIndex int
	snapshotLastTerm  int

	// Volatile state
	state       State
	commitIndex int
	lastApplied int

	// Leader-only volatile state (re-initialised on each election win)
	nextIndex  []int
	matchIndex []int

	// Simple key/value state machine (applied entries update this)
	stateMachine map[string]string

	// Channels
	stopCh      chan struct{}
	commandCh   chan string        // submit a command to the leader
	applyCh     chan ApplyMsg      // committed entries delivered to caller
	resetTimer  chan struct{}      // follower resets its election timer on this

	// Internal — used by tests
	killed bool
}

// ApplyMsg is delivered on the applyCh channel when an entry is committed.
type ApplyMsg struct {
	CommandValid bool
	Command      string
	CommandIndex int
	// Snapshot fields
	SnapshotValid bool
	Snapshot      []byte
	SnapshotIndex int
	SnapshotTerm  int
}

// NodeStatus is a read-only snapshot of a node's current state for HTTP/test use.
type NodeStatus struct {
	ID          int
	State       string
	Term        int
	CommitIndex int
	LogLen      int
	LeaderID    int
}

// ── v0 — Leader election ──────────────────────────────────────────────────────

// NewNode creates a Node with the given id.  Peers must be set before calling Start.
func NewNode(id int) *Node {
	return &Node{
		id:           id,
		currentTerm:  0,
		votedFor:     -1,
		log:          []LogEntry{},
		state:        Follower,
		commitIndex:  0,
		lastApplied:  0,
		stateMachine: make(map[string]string),
		stopCh:       make(chan struct{}),
		commandCh:    make(chan string, 64),
		applyCh:      make(chan ApplyMsg, 64),
		resetTimer:   make(chan struct{}, 1),
	}
}

// SetPeers wires up the node's peer references (must be called before Start).
func (n *Node) SetPeers(peers []*Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.peers = peers
	n.nextIndex = make([]int, len(peers))
	n.matchIndex = make([]int, len(peers))
}

// Start launches the node's main goroutine.
func (n *Node) Start() {
	go n.run()
}

// Stop shuts the node down.
func (n *Node) Stop() {
	n.mu.Lock()
	n.killed = true
	n.mu.Unlock()
	close(n.stopCh)
}

// ApplyCh returns the channel on which committed entries are delivered.
func (n *Node) ApplyCh() <-chan ApplyMsg {
	return n.applyCh
}

// Submit enqueues a command for the leader to append.  Returns false if the node
// is not the leader.
func (n *Node) Submit(command string) bool {
	n.mu.Lock()
	if n.state != Leader {
		n.mu.Unlock()
		return false
	}
	n.mu.Unlock()
	select {
	case n.commandCh <- command:
		return true
	default:
		return false
	}
}

// Status returns a read-only snapshot of the node's state.
func (n *Node) Status() NodeStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	leaderID := -1
	if n.state == Leader {
		leaderID = n.id
	}
	return NodeStatus{
		ID:          n.id,
		State:       n.state.String(),
		Term:        n.currentTerm,
		CommitIndex: n.commitIndex,
		LogLen:      len(n.log),
		LeaderID:    leaderID,
	}
}

// StateMachineCopy returns a deep copy of the current key/value state machine.
func (n *Node) StateMachineCopy() map[string]string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make(map[string]string, len(n.stateMachine))
	for k, v := range n.stateMachine {
		out[k] = v
	}
	return out
}

// Log returns a copy of the in-memory log (entries after any snapshot).
func (n *Node) Log() []LogEntry {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]LogEntry, len(n.log))
	copy(out, n.log)
	return out
}

// IsLeader reports whether the node currently believes itself to be the leader.
func (n *Node) IsLeader() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.state == Leader
}

// randomTimeout returns a random duration in [electionTimeoutMin, electionTimeoutMax].
func randomTimeout() time.Duration {
	spread := int64(electionTimeoutMax - electionTimeoutMin)
	return electionTimeoutMin + time.Duration(rand.Int63n(spread))
}

// run is the node's main event loop.
func (n *Node) run() {
	for {
		n.mu.Lock()
		state := n.state
		n.mu.Unlock()

		switch state {
		case Follower:
			n.runFollower()
		case Candidate:
			n.runCandidate()
		case Leader:
			n.runLeader()
		}

		select {
		case <-n.stopCh:
			return
		default:
		}
	}
}

// runFollower waits for an election timeout.  If the timer fires before a
// heartbeat resets it, the node becomes a Candidate.
func (n *Node) runFollower() {
	timer := time.NewTimer(randomTimeout())
	defer timer.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-n.resetTimer:
			// Received heartbeat or valid RPC; reset the timeout.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(randomTimeout())
		case <-timer.C:
			// Timeout fired — become Candidate.
			n.mu.Lock()
			n.state = Candidate
			n.mu.Unlock()
			return
		}
	}
}

// runCandidate starts an election and collects votes.
func (n *Node) runCandidate() {
	n.mu.Lock()
	n.currentTerm++
	n.votedFor = n.id
	term := n.currentTerm
	lastLogIndex, lastLogTerm := n.lastLogInfo()
	peers := n.peers
	n.mu.Unlock()

	votes := 1 // vote for self
	majority := (len(peers)+1)/2 + 1

	voteReplyCh := make(chan RequestVoteReply, len(peers))

	// Send RequestVote to all peers in parallel.
	for _, peer := range peers {
		go func(p *Node) {
			args := RequestVoteArgs{
				Term:         term,
				CandidateID:  n.id,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			}
			reply := RequestVoteReply{}
			p.HandleRequestVote(args, &reply)
			voteReplyCh <- reply
		}(peer)
	}

	timer := time.NewTimer(randomTimeout())
	defer timer.Stop()

	for {
		select {
		case <-n.stopCh:
			return
		case <-timer.C:
			// Split vote; try again.
			return
		case reply := <-voteReplyCh:
			n.mu.Lock()
			// If a higher term is seen, revert to Follower.
			if reply.Term > n.currentTerm {
				n.currentTerm = reply.Term
				n.votedFor = -1
				n.state = Follower
				n.mu.Unlock()
				return
			}
			if reply.VoteGranted && n.state == Candidate {
				votes++
				if votes >= majority {
					n.becomeLeader()
					n.mu.Unlock()
					return
				}
			}
			n.mu.Unlock()
		}
	}
}

// becomeLeader transitions the node to Leader state and initialises leader fields.
// Must be called with n.mu held.
func (n *Node) becomeLeader() {
	n.state = Leader
	logLen := n.snapshotLastIndex + len(n.log)
	for i := range n.nextIndex {
		n.nextIndex[i] = logLen + 1
		n.matchIndex[i] = 0
	}
}

// runLeader sends heartbeats and processes commands while it is the leader.
func (n *Node) runLeader() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// Send an immediate heartbeat to assert leadership.
	n.broadcastAppendEntries()

	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.Lock()
			if n.state != Leader {
				n.mu.Unlock()
				return
			}
			n.mu.Unlock()
			n.broadcastAppendEntries()
		case cmd := <-n.commandCh:
			n.mu.Lock()
			if n.state != Leader {
				n.mu.Unlock()
				return
			}
			// v1: append the command to the leader's local log.
			entry := LogEntry{Term: n.currentTerm, Command: cmd}
			n.log = append(n.log, entry)
			n.mu.Unlock()
			n.broadcastAppendEntries()
		}
	}
}

// HandleRequestVote is called by a Candidate (via goroutine) on this node.
func (n *Node) HandleRequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	reply.Term = n.currentTerm
	reply.VoteGranted = false

	// Reject stale terms.
	if args.Term < n.currentTerm {
		return
	}

	// Update term if necessary.
	if args.Term > n.currentTerm {
		n.currentTerm = args.Term
		n.votedFor = -1
		n.state = Follower
		n.tryResetTimer()
	}

	// Only vote if we haven't voted yet (or already voted for this candidate).
	if n.votedFor != -1 && n.votedFor != args.CandidateID {
		return
	}

	// Election safety: only vote for a candidate whose log is at least as
	// up-to-date as ours (last log term, then last log index).
	myLastIndex, myLastTerm := n.lastLogInfo()
	if args.LastLogTerm < myLastTerm {
		return
	}
	if args.LastLogTerm == myLastTerm && args.LastLogIndex < myLastIndex {
		return
	}

	n.votedFor = args.CandidateID
	reply.VoteGranted = true
	n.tryResetTimer()
}

// HandleAppendEntries is called by the Leader on this node.
func (n *Node) HandleAppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	reply.Term = n.currentTerm
	reply.Success = false

	if args.Term < n.currentTerm {
		return
	}

	// Revert to Follower on a valid AppendEntries from a current or newer term.
	if args.Term > n.currentTerm {
		n.currentTerm = args.Term
		n.votedFor = -1
	}
	n.state = Follower
	n.tryResetTimer()

	// ── v1: Log consistency check ─────────────────────────────────────────
	// prevLogIndex is relative to the absolute log (including snapshotted entries).
	// We translate it to our local log slice using snapshotLastIndex.
	if args.PrevLogIndex > 0 {
		// The entry at PrevLogIndex must exist and have the right term.
		localIdx := args.PrevLogIndex - n.snapshotLastIndex - 1
		if localIdx < -1 {
			// This entry is within the snapshot; we treat it as consistent.
		} else if localIdx == -1 {
			// The previous entry is exactly the last snapshotted entry.
			if args.PrevLogTerm != n.snapshotLastTerm {
				reply.ConflictTerm = n.snapshotLastTerm
				reply.ConflictIndex = n.snapshotLastIndex
				return
			}
		} else if localIdx >= len(n.log) {
			// We don't have this entry yet.
			reply.ConflictIndex = n.snapshotLastIndex + len(n.log)
			return
		} else if n.log[localIdx].Term != args.PrevLogTerm {
			// Term mismatch — find the first index of the conflicting term.
			conflictTerm := n.log[localIdx].Term
			reply.ConflictTerm = conflictTerm
			ci := localIdx
			for ci > 0 && n.log[ci-1].Term == conflictTerm {
				ci--
			}
			reply.ConflictIndex = n.snapshotLastIndex + ci + 1
			return
		}
	}

	// Append new entries, removing any conflicting tail.
	insertAt := args.PrevLogIndex - n.snapshotLastIndex
	if insertAt < 0 {
		insertAt = 0
	}
	for i, entry := range args.Entries {
		idx := insertAt + i
		if idx < len(n.log) {
			if n.log[idx].Term != entry.Term {
				// Conflict: truncate and append.
				n.log = n.log[:idx]
				n.log = append(n.log, args.Entries[i:]...)
				break
			}
			// Already have this entry; skip.
		} else {
			n.log = append(n.log, args.Entries[i:]...)
			break
		}
	}

	// Advance commitIndex.
	if args.LeaderCommit > n.commitIndex {
		lastNewIndex := n.snapshotLastIndex + len(n.log)
		if args.LeaderCommit < lastNewIndex {
			n.commitIndex = args.LeaderCommit
		} else {
			n.commitIndex = lastNewIndex
		}
		n.applyCommitted()
	}

	reply.Success = true
}

// ── v2: Snapshot RPC ──────────────────────────────────────────────────────────

// HandleInstallSnapshot installs a snapshot sent by the leader.
func (n *Node) HandleInstallSnapshot(args InstallSnapshotArgs, reply *InstallSnapshotReply) {
	n.mu.Lock()
	defer n.mu.Unlock()

	reply.Term = n.currentTerm
	if args.Term < n.currentTerm {
		return
	}
	if args.Term > n.currentTerm {
		n.currentTerm = args.Term
		n.votedFor = -1
		n.state = Follower
	}
	n.tryResetTimer()

	// Ignore stale snapshots.
	if args.LastIncludedIndex <= n.snapshotLastIndex {
		return
	}

	// Decode the snapshot.
	var snap Snapshot
	if err := json.Unmarshal(args.Data, &snap); err != nil {
		return
	}

	// Retain log entries after the snapshot index.
	localOffset := args.LastIncludedIndex - n.snapshotLastIndex
	if localOffset >= len(n.log) {
		n.log = []LogEntry{}
	} else {
		n.log = n.log[localOffset:]
	}

	n.snapshotLastIndex = args.LastIncludedIndex
	n.snapshotLastTerm = args.LastIncludedTerm
	n.snapshot = &snap
	n.stateMachine = snap.StateMachine
	n.commitIndex = args.LastIncludedIndex
	n.lastApplied = args.LastIncludedIndex

	// Notify apply channel.
	select {
	case n.applyCh <- ApplyMsg{
		SnapshotValid: true,
		Snapshot:      args.Data,
		SnapshotIndex: args.LastIncludedIndex,
		SnapshotTerm:  args.LastIncludedTerm,
	}:
	default:
	}
}

// ── Leader helpers ────────────────────────────────────────────────────────────

// broadcastAppendEntries sends AppendEntries to all peers.
func (n *Node) broadcastAppendEntries() {
	n.mu.Lock()
	peers := n.peers
	term := n.currentTerm
	leaderID := n.id
	commitIndex := n.commitIndex
	n.mu.Unlock()

	for i, peer := range peers {
		go func(peerIdx int, p *Node) {
			n.mu.Lock()
			if n.state != Leader {
				n.mu.Unlock()
				return
			}

			ni := n.nextIndex[peerIdx]
			// If nextIndex is behind the snapshot, send InstallSnapshot instead.
			if ni <= n.snapshotLastIndex {
				if n.snapshot != nil {
					data, err := json.Marshal(n.snapshot)
					if err == nil {
						args := InstallSnapshotArgs{
							Term:              term,
							LeaderID:          leaderID,
							LastIncludedIndex: n.snapshotLastIndex,
							LastIncludedTerm:  n.snapshotLastTerm,
							Data:              data,
						}
						n.mu.Unlock()
						reply := InstallSnapshotReply{}
						p.HandleInstallSnapshot(args, &reply)
						n.mu.Lock()
						if reply.Term > n.currentTerm {
							n.currentTerm = reply.Term
							n.votedFor = -1
							n.state = Follower
						} else {
							n.nextIndex[peerIdx] = n.snapshotLastIndex + 1
							n.matchIndex[peerIdx] = n.snapshotLastIndex
						}
						n.mu.Unlock()
						return
					}
				}
				n.mu.Unlock()
				return
			}

			prevLogIndex := ni - 1
			prevLogTerm := 0
			if prevLogIndex > n.snapshotLastIndex {
				localPrev := prevLogIndex - n.snapshotLastIndex - 1
				if localPrev >= 0 && localPrev < len(n.log) {
					prevLogTerm = n.log[localPrev].Term
				}
			} else if prevLogIndex == n.snapshotLastIndex {
				prevLogTerm = n.snapshotLastTerm
			}

			localStart := ni - n.snapshotLastIndex - 1
			var entries []LogEntry
			if localStart >= 0 && localStart < len(n.log) {
				entries = make([]LogEntry, len(n.log)-localStart)
				copy(entries, n.log[localStart:])
			}
			n.mu.Unlock()

			args := AppendEntriesArgs{
				Term:         term,
				LeaderID:     leaderID,
				PrevLogIndex: prevLogIndex,
				PrevLogTerm:  prevLogTerm,
				Entries:      entries,
				LeaderCommit: commitIndex,
			}
			reply := AppendEntriesReply{}
			p.HandleAppendEntries(args, &reply)

			n.mu.Lock()
			defer n.mu.Unlock()

			if reply.Term > n.currentTerm {
				n.currentTerm = reply.Term
				n.votedFor = -1
				n.state = Follower
				return
			}
			if n.state != Leader {
				return
			}
			if reply.Success {
				newMatch := prevLogIndex + len(entries)
				if newMatch > n.matchIndex[peerIdx] {
					n.matchIndex[peerIdx] = newMatch
				}
				n.nextIndex[peerIdx] = n.matchIndex[peerIdx] + 1
				n.advanceCommitIndex()
			} else {
				// Decrement nextIndex and retry next heartbeat cycle.
				if reply.ConflictIndex > 0 {
					n.nextIndex[peerIdx] = reply.ConflictIndex
				} else if n.nextIndex[peerIdx] > 1 {
					n.nextIndex[peerIdx]--
				}
			}
		}(i, peer)
	}
}

// advanceCommitIndex advances commitIndex to the highest index replicated on a
// majority of servers.  Must be called with n.mu held.
func (n *Node) advanceCommitIndex() {
	// Find the highest N such that N > commitIndex, log[N].Term == currentTerm,
	// and a majority of matchIndex[] >= N.
	totalNodes := len(n.peers) + 1
	majority := totalNodes/2 + 1

	lastLog := n.snapshotLastIndex + len(n.log)
	for idx := lastLog; idx > n.commitIndex; idx-- {
		localIdx := idx - n.snapshotLastIndex - 1
		if localIdx < 0 || localIdx >= len(n.log) {
			continue
		}
		if n.log[localIdx].Term != n.currentTerm {
			// Raft safety: only commit entries from the current term.
			continue
		}
		// Count replicas.
		replicated := 1 // leader itself
		for _, mi := range n.matchIndex {
			if mi >= idx {
				replicated++
			}
		}
		if replicated >= majority {
			n.commitIndex = idx
			n.applyCommitted()
			// v2: check if we should take a snapshot.
			n.maybeSnapshot()
			break
		}
	}
}

// applyCommitted applies all newly committed entries to the state machine.
// Must be called with n.mu held.
func (n *Node) applyCommitted() {
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		localIdx := n.lastApplied - n.snapshotLastIndex - 1
		if localIdx < 0 || localIdx >= len(n.log) {
			continue
		}
		entry := n.log[localIdx]
		n.applyToStateMachine(entry.Command)

		msg := ApplyMsg{
			CommandValid: true,
			Command:      entry.Command,
			CommandIndex: n.lastApplied,
		}
		select {
		case n.applyCh <- msg:
		default:
		}
	}
}

// applyToStateMachine executes a command against the key/value state machine.
// Commands are simple strings in the form "SET key value" or "DEL key".
func (n *Node) applyToStateMachine(cmd string) {
	var op, key, val string
	// Parse simple "SET key value" or "DEL key" commands.
	parts := splitCommand(cmd)
	if len(parts) == 0 {
		return
	}
	op = parts[0]
	if len(parts) > 1 {
		key = parts[1]
	}
	if len(parts) > 2 {
		val = parts[2]
	}
	switch op {
	case "SET":
		if key != "" {
			n.stateMachine[key] = val
		}
	case "DEL":
		delete(n.stateMachine, key)
	}
}

// splitCommand splits a command string into at most 3 parts (op, key, value).
func splitCommand(cmd string) []string {
	result := make([]string, 0, 3)
	start := -1
	for i, ch := range cmd {
		if ch == ' ' || ch == '\t' {
			if start != -1 {
				result = append(result, cmd[start:i])
				start = -1
				if len(result) == 2 {
					// The third part (value) may contain spaces; take the rest.
					rest := cmd[i+1:]
					if rest != "" {
						result = append(result, rest)
					}
					return result
				}
			}
		} else {
			if start == -1 {
				start = i
			}
		}
	}
	if start != -1 {
		result = append(result, cmd[start:])
	}
	return result
}

// ── v2: Snapshot helpers ──────────────────────────────────────────────────────

// maybeSnapshot takes a snapshot if the log has grown past the threshold.
// Must be called with n.mu held.
func (n *Node) maybeSnapshot() {
	if len(n.log) <= snapshotThreshold {
		return
	}
	// Snapshot at commitIndex.
	commitLocal := n.commitIndex - n.snapshotLastIndex - 1
	if commitLocal < 0 || commitLocal >= len(n.log) {
		return
	}
	sm := make(map[string]string, len(n.stateMachine))
	for k, v := range n.stateMachine {
		sm[k] = v
	}
	snap := &Snapshot{
		LastIncludedIndex: n.commitIndex,
		LastIncludedTerm:  n.log[commitLocal].Term,
		StateMachine:      sm,
	}
	// Truncate the log up to (and including) commitIndex.
	n.log = n.log[commitLocal+1:]
	n.snapshotLastIndex = snap.LastIncludedIndex
	n.snapshotLastTerm = snap.LastIncludedTerm
	n.snapshot = snap

	// Force followers behind snapshotLastIndex to receive InstallSnapshot
	// on the next heartbeat cycle (handled in broadcastAppendEntries).
}

// lastLogInfo returns the index and term of the last log entry, accounting for
// the snapshot offset.  Must be called with n.mu held.
func (n *Node) lastLogInfo() (index, term int) {
	if len(n.log) == 0 {
		return n.snapshotLastIndex, n.snapshotLastTerm
	}
	idx := n.snapshotLastIndex + len(n.log)
	return idx, n.log[len(n.log)-1].Term
}

// tryResetTimer non-blockingly signals the follower loop to reset its timer.
// Must be called with n.mu held.
func (n *Node) tryResetTimer() {
	select {
	case n.resetTimer <- struct{}{}:
	default:
	}
}

// ── Cluster helper ────────────────────────────────────────────────────────────

// Cluster manages a set of Raft nodes wired together over in-process channels.
type Cluster struct {
	Nodes []*Node
}

// NewCluster creates an n-node in-process Raft cluster and starts all nodes.
func NewCluster(n int) *Cluster {
	nodes := make([]*Node, n)
	for i := range nodes {
		nodes[i] = NewNode(i)
	}
	for i, node := range nodes {
		peers := make([]*Node, 0, n-1)
		for j, other := range nodes {
			if j != i {
				peers = append(peers, other)
			}
		}
		node.SetPeers(peers)
	}
	for _, node := range nodes {
		node.Start()
	}
	return &Cluster{Nodes: nodes}
}

// WaitForLeader blocks until a leader is elected or the deadline passes.
// Returns the leader node and whether one was found within the deadline.
func (c *Cluster) WaitForLeader(timeout time.Duration) (*Node, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, n := range c.Nodes {
			if n.IsLeader() {
				return n, true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, false
}

// Leader returns the current leader node, or nil if there is none.
func (c *Cluster) Leader() *Node {
	for _, n := range c.Nodes {
		if n.IsLeader() {
			return n
		}
	}
	return nil
}

// StopAll stops all nodes in the cluster.
func (c *Cluster) StopAll() {
	for _, n := range c.Nodes {
		n.Stop()
	}
}
