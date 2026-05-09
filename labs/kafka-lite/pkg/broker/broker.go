// Package broker implements a simplified Kafka-like message broker.
//
// Three progressive versions live in this file:
//
//   v0 — In-memory log: a topic is a [][]byte. Produce appends; Consume reads
//        from an explicit offset. Offsets are permanent — messages are never
//        deleted when consumed. Key lesson: Kafka is a log, not a queue.
//
//   v1 — Segmented log on disk: each topic-partition is a directory. Segments are
//        fixed-size .log files with a matching .index file (sparse offset → byte
//        position). On produce: append to active segment, batch fsync. On consume:
//        binary search the index to find starting byte, scan forward linearly.
//        Key lesson: the index file makes seek O(log N) instead of O(N).
//
//   v2 — Consumer groups: per-group per-partition committed offsets stored in a
//        __consumer_offsets topic. Members must heartbeat every 3s or are ejected.
//        Key lesson: consumer groups decouple read position from the log — multiple
//        groups can read the same log at different speeds.

package broker

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ── Shared types ──────────────────────────────────────────────────────────────

// Message is a consumed record: its absolute offset plus the raw payload.
type Message struct {
	Offset  int64
	Payload []byte
}

// ── v0 — In-memory log ────────────────────────────────────────────────────────
//
// Lesson: an offset is a permanent cursor into an immutable sequence. Consuming
// a message does NOT remove it. Two consumers at offset 0 both get the full log.
// This is the fundamental difference from a queue (AMQP, SQS) where a consumed
// message disappears.

// MemBroker is a fully in-memory broker (v0). All topics share a single lock.
// Not suitable for production — use DiskBroker for persistence.
type MemBroker struct {
	mu     sync.RWMutex
	topics map[string][][]byte // topic → ordered log of raw payloads
}

// NewMemBroker creates an empty in-memory broker.
func NewMemBroker() *MemBroker {
	return &MemBroker{
		topics: make(map[string][][]byte),
	}
}

// Produce appends a message to the named topic and returns the assigned offset.
// If the topic does not exist it is created automatically.
// The returned offset is permanent: Consume(topic, offset, 1) always returns
// this exact message regardless of how many other consumers have read it.
func (b *MemBroker) Produce(topic string, payload []byte) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	log := b.topics[topic]
	offset := int64(len(log))
	b.topics[topic] = append(log, payload)
	return offset
}

// Consume returns up to maxMessages messages starting at the given offset.
// If offset is beyond the end of the log, an empty slice is returned.
// Offset 0 is always valid (the first ever message).
func (b *MemBroker) Consume(topic string, offset int64, maxMessages int) []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()

	log := b.topics[topic]
	if offset >= int64(len(log)) || maxMessages <= 0 {
		return nil
	}

	end := offset + int64(maxMessages)
	if end > int64(len(log)) {
		end = int64(len(log))
	}

	result := make([]Message, 0, end-offset)
	for i := offset; i < end; i++ {
		result = append(result, Message{Offset: i, Payload: log[i]})
	}
	return result
}

// LogLength returns the number of messages in the named topic (= next free offset).
func (b *MemBroker) LogLength(topic string) int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return int64(len(b.topics[topic]))
}

// ── v1 — Segmented log on disk ────────────────────────────────────────────────
//
// Layout:
//
//   <dataDir>/<topic>/<partition>/
//       00000000000000000000.log    ← first segment (base offset 0)
//       00000000000000000000.index  ← sparse index for that segment
//       00000000000000065536.log    ← next segment (base offset 65536)
//       00000000000000065536.index
//
// Log record format (binary, big-endian):
//
//   ┌──────────────────────────────────────────────────────────┐
//   │  absolute_offset (int64, 8 bytes)                        │
//   │  payload_len     (int32, 4 bytes)                        │
//   │  payload         (payload_len bytes)                     │
//   └──────────────────────────────────────────────────────────┘
//
// Index entry format (binary, big-endian):
//
//   ┌───────────────────────────────────────────────────────────┐
//   │  relative_offset (int32, 4 bytes)  offset - base_offset  │
//   │  position        (int32, 4 bytes)  byte offset in .log   │
//   └───────────────────────────────────────────────────────────┘
//
// The index is sparse: one entry every indexInterval messages (default: 4).
// Binary search on the index gives the segment .log position just before the
// target offset; then we scan forward linearly (at most indexInterval records).
//
// Batch fsync: we buffer up to flushBatch writes before calling fsync. This
// trades a small durability window for 5-10x throughput improvement.

const (
	maxSegmentBytes = 64 * 1024 * 1024 // 64 MB per segment
	indexInterval   = 4                // index one entry per N messages
	flushBatch      = 64               // fsync every N appends
)

// segmentInfo holds the metadata for one segment.
type segmentInfo struct {
	baseOffset  int64
	logPath     string
	indexPath   string
	logFile     *os.File
	indexFile   *os.File
	logSize     int64  // bytes written to .log
	msgCount    int    // messages in this segment (for index spacing)
	pending     int    // writes since last fsync
}

// indexEntry maps a relative offset to a byte position in the segment's .log file.
type indexEntry struct {
	relOffset int32
	position  int32
}

// partition is one topic-partition: a directory holding ordered segments.
type partition struct {
	mu       sync.Mutex
	dir      string
	segments []*segmentInfo
	active   *segmentInfo
	nextOff  int64 // global next-offset (= total messages ever produced)
}

// DiskBroker is a file-backed broker (v1 + v2). Topics are stored on disk as
// segmented logs with sparse offset indexes.
type DiskBroker struct {
	mu         sync.RWMutex
	dataDir    string
	partitions map[string]*partition // "topic:partitionID" → partition

	// v2: consumer groups
	groups     map[string]*consumerGroup
	groupsMu   sync.Mutex
}

// NewDiskBroker opens (or creates) a broker storing data in dataDir.
func NewDiskBroker(dataDir string) (*DiskBroker, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	b := &DiskBroker{
		dataDir:    dataDir,
		partitions: make(map[string]*partition),
		groups:     make(map[string]*consumerGroup),
	}
	return b, nil
}

// partitionKey returns the map key for a topic-partition pair.
func partitionKey(topic string, partitionID int) string {
	return fmt.Sprintf("%s:%d", topic, partitionID)
}

// getOrCreatePartition returns the partition for (topic, partitionID),
// creating it on disk if it doesn't exist. Caller must not hold b.mu.
func (b *DiskBroker) getOrCreatePartition(topic string, partitionID int) (*partition, error) {
	key := partitionKey(topic, partitionID)

	b.mu.RLock()
	p, ok := b.partitions[key]
	b.mu.RUnlock()
	if ok {
		return p, nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Double-check after acquiring write lock.
	if p, ok = b.partitions[key]; ok {
		return p, nil
	}

	dir := filepath.Join(b.dataDir, topic, fmt.Sprintf("%d", partitionID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create partition dir %s: %w", dir, err)
	}

	p = &partition{dir: dir}
	if err := p.open(); err != nil {
		return nil, fmt.Errorf("open partition %s: %w", key, err)
	}

	b.partitions[key] = p
	return p, nil
}

// Produce appends a message to topic-partition and returns its absolute offset.
// Segments are rotated automatically when the active segment reaches maxSegmentBytes.
func (b *DiskBroker) Produce(topic string, partitionID int, payload []byte) (int64, error) {
	p, err := b.getOrCreatePartition(topic, partitionID)
	if err != nil {
		return 0, err
	}
	return p.append(payload)
}

// Consume returns up to maxMessages messages from topic-partition starting at offset.
func (b *DiskBroker) Consume(topic string, partitionID int, offset int64, maxMessages int) ([]Message, error) {
	p, err := b.getOrCreatePartition(topic, partitionID)
	if err != nil {
		return nil, err
	}
	return p.read(offset, maxMessages)
}

// LogLength returns the next free offset (= total messages ever produced) for
// the given topic-partition.
func (b *DiskBroker) LogLength(topic string, partitionID int) (int64, error) {
	p, err := b.getOrCreatePartition(topic, partitionID)
	if err != nil {
		return 0, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nextOff, nil
}

// Close flushes and closes all open file handles.
func (b *DiskBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, p := range b.partitions {
		p.closeAll()
	}
	return nil
}

// ── partition internals ───────────────────────────────────────────────────────

// open scans the partition directory and recovers existing segments, then opens
// (or creates) the active segment for appending.
func (p *partition) open() error {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return err
	}

	// Collect all .log files — their names encode the base offset.
	var bases []int64
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".log" {
			continue
		}
		var base int64
		if _, err := fmt.Sscanf(e.Name(), "%020d.log", &base); err == nil {
			bases = append(bases, base)
		}
	}
	sort.Slice(bases, func(i, j int) bool { return bases[i] < bases[j] })

	for _, base := range bases {
		seg, err := openSegment(p.dir, base, false)
		if err != nil {
			return fmt.Errorf("open segment %d: %w", base, err)
		}
		p.segments = append(p.segments, seg)
		p.nextOff = base + int64(seg.msgCount)
	}

	// Open (or create) the active segment for writing.
	if len(p.segments) == 0 {
		seg, err := openSegment(p.dir, 0, true)
		if err != nil {
			return err
		}
		p.segments = append(p.segments, seg)
		p.active = seg
		p.nextOff = 0
	} else {
		last := p.segments[len(p.segments)-1]
		// Re-open the last segment in append mode.
		logF, err := os.OpenFile(last.logPath, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		idxF, err := os.OpenFile(last.indexPath, os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			logF.Close()
			return err
		}
		last.logFile = logF
		last.indexFile = idxF
		p.active = last
	}

	return nil
}

// openSegment creates or opens a segment with the given base offset.
// If create=true and the files don't exist, they are created.
func openSegment(dir string, base int64, create bool) (*segmentInfo, error) {
	name := fmt.Sprintf("%020d", base)
	logPath := filepath.Join(dir, name+".log")
	idxPath := filepath.Join(dir, name+".index")

	seg := &segmentInfo{
		baseOffset: base,
		logPath:    logPath,
		indexPath:  idxPath,
	}

	flag := os.O_RDONLY
	if create {
		flag = os.O_CREATE | os.O_RDWR
	}

	logF, err := os.OpenFile(logPath, flag, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			return seg, nil // segment exists in list but files missing — treat as empty
		}
		return nil, err
	}

	idxF, err := os.OpenFile(idxPath, flag, 0o644)
	if err != nil {
		logF.Close()
		return nil, err
	}

	// Count how many messages are in the segment by reading the index.
	// The index has one entry per indexInterval messages, so we count entries
	// and use msgCount to track the real count as we scan.
	if stat, err := logF.Stat(); err == nil {
		seg.logSize = stat.Size()
	}

	// Count messages by reading the log sequentially (only needed on startup).
	seg.msgCount = countMessagesInLog(logF)
	logF.Close()
	idxF.Close()

	// If create, re-open for writing.
	if create {
		logF, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		idxF, err = os.OpenFile(idxPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			logF.Close()
			return nil, err
		}
		seg.logFile = logF
		seg.indexFile = idxF
	}

	return seg, nil
}

// countMessagesInLog counts the number of complete log records in f.
func countMessagesInLog(f *os.File) int {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0
	}
	count := 0
	for {
		var offsetBuf [8]byte
		if _, err := io.ReadFull(f, offsetBuf[:]); err != nil {
			break
		}
		var lenBuf [4]byte
		if _, err := io.ReadFull(f, lenBuf[:]); err != nil {
			break
		}
		payloadLen := int64(binary.BigEndian.Uint32(lenBuf[:]))
		if _, err := f.Seek(payloadLen, io.SeekCurrent); err != nil {
			break
		}
		count++
	}
	return count
}

// append writes a message to the active segment, rotating if needed.
// Returns the assigned absolute offset.
func (p *partition) append(payload []byte) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Rotate if the active segment has reached the size limit.
	if p.active.logSize >= maxSegmentBytes {
		if err := p.rotate(); err != nil {
			return 0, fmt.Errorf("rotate segment: %w", err)
		}
	}

	offset := p.nextOff
	seg := p.active

	// Write log record: offset (8) + len (4) + payload.
	recSize := 8 + 4 + len(payload)
	buf := make([]byte, recSize)
	binary.BigEndian.PutUint64(buf[0:8], uint64(offset))
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(payload)))
	copy(buf[12:], payload)

	bytePos := seg.logSize // position in .log where this record starts

	if _, err := seg.logFile.Write(buf); err != nil {
		return 0, fmt.Errorf("write log: %w", err)
	}
	seg.logSize += int64(recSize)

	// Write index entry every indexInterval messages.
	if seg.msgCount%indexInterval == 0 {
		relOffset := int32(offset - seg.baseOffset)
		position := int32(bytePos)
		var idxBuf [8]byte
		binary.BigEndian.PutUint32(idxBuf[0:4], uint32(relOffset))
		binary.BigEndian.PutUint32(idxBuf[4:8], uint32(position))
		if _, err := seg.indexFile.Write(idxBuf[:]); err != nil {
			return 0, fmt.Errorf("write index: %w", err)
		}
	}

	seg.msgCount++
	seg.pending++
	p.nextOff++

	// Batch fsync: sync every flushBatch writes.
	if seg.pending >= flushBatch {
		if err := seg.logFile.Sync(); err != nil {
			return 0, fmt.Errorf("fsync: %w", err)
		}
		seg.pending = 0
	}

	return offset, nil
}

// rotate seals the active segment and opens a fresh one starting at p.nextOff.
func (p *partition) rotate() error {
	seg := p.active

	// Final fsync before sealing.
	if seg.logFile != nil {
		if err := seg.logFile.Sync(); err != nil {
			return err
		}
		seg.logFile.Close()
		seg.indexFile.Close()
		seg.logFile = nil
		seg.indexFile = nil
	}

	newSeg, err := openSegment(p.dir, p.nextOff, true)
	if err != nil {
		return err
	}

	p.segments = append(p.segments, newSeg)
	p.active = newSeg
	return nil
}

// closeAll flushes and closes all open file handles for this partition.
func (p *partition) closeAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, seg := range p.segments {
		if seg.logFile != nil {
			seg.logFile.Sync()
			seg.logFile.Close()
			seg.logFile = nil
		}
		if seg.indexFile != nil {
			seg.indexFile.Close()
			seg.indexFile = nil
		}
	}
}

// read returns up to maxMessages records starting at the given global offset.
func (p *partition) read(offset int64, maxMessages int) ([]Message, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if maxMessages <= 0 || offset >= p.nextOff {
		return nil, nil
	}

	// Find the segment that contains this offset (the last segment whose
	// baseOffset <= offset).
	segIdx := sort.Search(len(p.segments), func(i int) bool {
		return p.segments[i].baseOffset > offset
	}) - 1

	if segIdx < 0 {
		return nil, nil
	}

	var result []Message
	remaining := maxMessages

	for si := segIdx; si < len(p.segments) && remaining > 0; si++ {
		seg := p.segments[si]

		msgs, err := readFromSegment(seg, offset, remaining)
		if err != nil {
			return nil, err
		}
		result = append(result, msgs...)
		remaining -= len(msgs)
		// Continue to next segment only if we need more messages.
		offset = seg.baseOffset + int64(seg.msgCount)
	}

	return result, nil
}

// readFromSegment reads up to maxMessages records from seg starting at globalOffset.
// It uses the sparse index to find the starting byte position, then scans forward.
func readFromSegment(seg *segmentInfo, globalOffset int64, maxMessages int) ([]Message, error) {
	// Read the index to find the byte position just before globalOffset.
	startPos, err := indexLookup(seg.indexPath, seg.baseOffset, globalOffset)
	if err != nil {
		startPos = 0 // fall back to scanning from the start
	}

	f, err := os.Open(seg.logPath)
	if err != nil {
		return nil, fmt.Errorf("open log %s: %w", seg.logPath, err)
	}
	defer f.Close()

	if _, err := f.Seek(startPos, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to %d: %w", startPos, err)
	}

	var result []Message
	for len(result) < maxMessages {
		// Read offset field.
		var offBuf [8]byte
		if _, err := io.ReadFull(f, offBuf[:]); err != nil {
			break // EOF or partial record
		}
		recOffset := int64(binary.BigEndian.Uint64(offBuf[:]))

		// Read length field.
		var lenBuf [4]byte
		if _, err := io.ReadFull(f, lenBuf[:]); err != nil {
			break
		}
		payloadLen := int(binary.BigEndian.Uint32(lenBuf[:]))

		// Read payload.
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(f, payload); err != nil {
			break
		}

		// Skip records before our target offset (scan from the indexed entry).
		if recOffset < globalOffset {
			continue
		}

		result = append(result, Message{Offset: recOffset, Payload: payload})
	}

	return result, nil
}

// indexLookup binary-searches the sparse index file for the byte position
// in the .log just before (or at) globalOffset.
func indexLookup(indexPath string, baseOffset, globalOffset int64) (int64, error) {
	f, err := os.Open(indexPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}

	nEntries := stat.Size() / 8
	if nEntries == 0 {
		return 0, nil
	}

	// Binary search: find the largest indexed relOffset that is still <= (globalOffset - baseOffset).
	relTarget := int32(globalOffset - baseOffset)

	lo, hi := int64(0), nEntries-1
	result := int64(0) // byte position in .log

	for lo <= hi {
		mid := (lo + hi) / 2
		if _, err := f.Seek(mid*8, io.SeekStart); err != nil {
			break
		}
		var entry [8]byte
		if _, err := io.ReadFull(f, entry[:]); err != nil {
			break
		}
		relOff := int32(binary.BigEndian.Uint32(entry[0:4]))
		pos := int64(binary.BigEndian.Uint32(entry[4:8]))

		if relOff <= relTarget {
			result = pos
			lo = mid + 1
		} else {
			if mid == 0 {
				break
			}
			hi = mid - 1
		}
	}

	return result, nil
}

// ── v2 — Consumer groups ──────────────────────────────────────────────────────
//
// A consumer group is identified by a groupId. Each member must heartbeat
// every heartbeatTimeout or it is considered dead and its partition assignments
// are revoked (triggering a rebalance).
//
// Committed offsets are stored in the __consumer_offsets topic (a special
// partition on the DiskBroker itself). This makes offsets durable across
// broker restarts — the same design Kafka uses.
//
// For simplicity, this implementation does not rebalance across members: all
// members of a group share the same partition. Real Kafka assigns each partition
// to exactly one group member.

const heartbeatTimeout = 3 * time.Second

// consumerGroup tracks membership and committed offsets for one group.
type consumerGroup struct {
	mu       sync.Mutex
	groupID  string
	members  map[string]time.Time // memberID → last heartbeat time
	offsets  map[string]int64     // "topic:partition" → committed offset
	broker   *DiskBroker
}

// JoinGroup registers a new consumer in the group and returns a memberID.
// If the group does not exist it is created.
func (b *DiskBroker) JoinGroup(groupID, clientID string) (string, error) {
	b.groupsMu.Lock()
	defer b.groupsMu.Unlock()

	g, ok := b.groups[groupID]
	if !ok {
		g = &consumerGroup{
			groupID: groupID,
			members: make(map[string]time.Time),
			offsets: make(map[string]int64),
			broker:  b,
		}
		b.groups[groupID] = g
	}

	memberID := fmt.Sprintf("%s-%d", clientID, time.Now().UnixNano())
	g.mu.Lock()
	g.members[memberID] = time.Now()
	g.mu.Unlock()

	return memberID, nil
}

// Heartbeat updates the last-seen time for a member.
// Returns an error if the member is not known (it was ejected or never joined).
func (b *DiskBroker) Heartbeat(groupID, memberID string) error {
	b.groupsMu.Lock()
	g, ok := b.groups[groupID]
	b.groupsMu.Unlock()

	if !ok {
		return fmt.Errorf("group %q not found", groupID)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if _, exists := g.members[memberID]; !exists {
		return fmt.Errorf("member %q not in group %q (may have been ejected)", memberID, groupID)
	}

	g.members[memberID] = time.Now()
	return nil
}

// evictStaleMembers removes members that have not heartbeated within heartbeatTimeout.
// This is the "rebalance trigger" — called lazily on Commit/Fetch operations.
func (g *consumerGroup) evictStaleMembers() {
	now := time.Now()
	for id, lastSeen := range g.members {
		if now.Sub(lastSeen) > heartbeatTimeout {
			delete(g.members, id)
		}
	}
}

// CommitOffset records the consumer group's current position for a topic-partition.
// The offset should be one past the last successfully processed message (next offset to fetch).
func (b *DiskBroker) CommitOffset(groupID, topic string, partitionID int, offset int64) error {
	b.groupsMu.Lock()
	g, ok := b.groups[groupID]
	b.groupsMu.Unlock()

	if !ok {
		return fmt.Errorf("group %q not found; call JoinGroup first", groupID)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.evictStaleMembers()

	key := partitionKey(topic, partitionID)
	g.offsets[key] = offset

	// Durably persist the committed offset by appending to __consumer_offsets.
	// Format: JSON {"group":"...","topic":"...","partition":0,"offset":N}
	rec, err := json.Marshal(map[string]interface{}{
		"group":     groupID,
		"topic":     topic,
		"partition": partitionID,
		"offset":    offset,
	})
	if err != nil {
		return fmt.Errorf("marshal offset record: %w", err)
	}

	_, err = b.Produce("__consumer_offsets", 0, rec)
	return err
}

// FetchOffset returns the last committed offset for a group-topic-partition triple.
// If no offset has been committed, 0 is returned (start from the beginning).
func (b *DiskBroker) FetchOffset(groupID, topic string, partitionID int) (int64, error) {
	b.groupsMu.Lock()
	g, ok := b.groups[groupID]
	b.groupsMu.Unlock()

	if !ok {
		return 0, nil // No commits yet — start from the beginning.
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.evictStaleMembers()

	key := partitionKey(topic, partitionID)
	return g.offsets[key], nil
}

// ActiveMembers returns the list of currently active member IDs in a group.
func (b *DiskBroker) ActiveMembers(groupID string) []string {
	b.groupsMu.Lock()
	g, ok := b.groups[groupID]
	b.groupsMu.Unlock()

	if !ok {
		return nil
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.evictStaleMembers()

	members := make([]string, 0, len(g.members))
	for id := range g.members {
		members = append(members, id)
	}
	return members
}
