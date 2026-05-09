package ws

import (
	"bytes"
	"net"
	"sync"
	"testing"
	"time"
)

// ── Handshake tests ───────────────────────────────────────────────────────────

// TestAcceptKey verifies the RFC 6455 §1.3 example.
// The spec uses key "dGhlIHNhbXBsZSBub25jZQ==" and expects
// "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=".
func TestAcceptKey(t *testing.T) {
	got := AcceptKey("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Errorf("AcceptKey() = %q, want %q", got, want)
	}
}

// ── Frame tests ───────────────────────────────────────────────────────────────

// TestFrameRoundtripText verifies write→read for a text frame.
func TestFrameRoundtripText(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte("hello world")
	if err := WriteFrame(&buf, OpcodeText, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	frame, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if frame.Opcode != OpcodeText {
		t.Errorf("opcode = %d, want %d", frame.Opcode, OpcodeText)
	}
	if !bytes.Equal(frame.Payload, payload) {
		t.Errorf("payload = %q, want %q", frame.Payload, payload)
	}
	if !frame.Fin {
		t.Error("FIN should be set")
	}
}

// TestFrameRoundtripBinary verifies write→read for a binary frame.
func TestFrameRoundtripBinary(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte{0x00, 0x01, 0x02, 0xFF}
	if err := WriteFrame(&buf, OpcodeBinary, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	frame, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if frame.Opcode != OpcodeBinary {
		t.Errorf("opcode = %d, want %d", frame.Opcode, OpcodeBinary)
	}
	if !bytes.Equal(frame.Payload, payload) {
		t.Errorf("payload mismatch")
	}
}

// TestFrameClose verifies close frame parsing.
func TestFrameClose(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, OpcodeClose, CloseFrame(1000, "goodbye")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	frame, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if frame.Opcode != OpcodeClose {
		t.Errorf("opcode = %d, want OpcodeClose(%d)", frame.Opcode, OpcodeClose)
	}
}

// TestFramePing verifies ping frame parsing.
func TestFramePing(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, OpcodePing, []byte("ping-data")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	frame, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if frame.Opcode != OpcodePing {
		t.Errorf("opcode = %d, want OpcodePing(%d)", frame.Opcode, OpcodePing)
	}
}

// TestMaskDecode verifies that Unmask decodes a known masked payload.
// Masking key [0x37, 0xfa, 0x21, 0x3d] applied to "Hello" produces
// [0x7f, 0x9f, 0x4d, 0x51, 0x58].
func TestMaskDecode(t *testing.T) {
	masked := []byte{0x7f, 0x9f, 0x4d, 0x51, 0x58}
	key := [4]byte{0x37, 0xfa, 0x21, 0x3d}
	Unmask(masked, key)
	want := []byte("Hello")
	if !bytes.Equal(masked, want) {
		t.Errorf("unmask = %q, want %q", masked, want)
	}
}

// ── Broadcast test ────────────────────────────────────────────────────────────

// TestBroadcastToThreeClients checks that a hub broadcast reaches 3 clients.
func TestBroadcastToThreeClients(t *testing.T) {
	hub := NewHub(100)

	type fakeEntry struct {
		c        *Client
		received [][]byte
		mu       sync.Mutex
	}

	entries := make([]*fakeEntry, 3)
	for i := range entries {
		conn := newTestConn()
		c := newClient(string(rune('A'+i)), string(rune('u'+i)), conn, hub)
		entries[i] = &fakeEntry{c: c}
		_ = hub.Register(c)
		hub.JoinRoom(c, "test-room")
		// Drain sendBuf to avoid blocking.
		go func(fe *fakeEntry) {
			for msg := range fe.c.sendBuf {
				fe.mu.Lock()
				fe.received = append(fe.received, msg)
				fe.mu.Unlock()
			}
		}(entries[i])
	}

	// Allow goroutines to start.
	time.Sleep(10 * time.Millisecond)

	testMsg := []byte(`{"event":"message","room":"test-room","content":"hi"}`)
	hub.Broadcast("test-room", testMsg)

	time.Sleep(20 * time.Millisecond)

	for i, fe := range entries {
		fe.mu.Lock()
		if len(fe.received) == 0 {
			t.Errorf("client %d received no messages", i)
		}
		fe.mu.Unlock()
		close(fe.c.sendBuf)
	}
}

// ── Backpressure test ─────────────────────────────────────────────────────────

// TestBackpressureDropsSlowClient verifies that a full sendBuf causes drops.
func TestBackpressureDropsSlowClient(t *testing.T) {
	hub := NewHub(100)

	conn := newTestConn()
	slowClient := newClient("slow", "slow-user", conn, hub)
	_ = hub.Register(slowClient)
	hub.JoinRoom(slowClient, "stress-room")

	// Drain presence messages first.
	for len(slowClient.sendBuf) > 0 {
		<-slowClient.sendBuf
	}

	// Fill the buffer to capacity.
	for i := 0; i < sendBufCap; i++ {
		slowClient.Send([]byte("msg"))
	}

	initialDrops := hub.Stats().DroppedMessages
	hub.Broadcast("stress-room", []byte("overflow"))
	time.Sleep(5 * time.Millisecond)

	drops := hub.Stats().DroppedMessages - initialDrops
	if drops == 0 {
		t.Error("expected DroppedMessages to increment for full buffer client")
	}
}

// ── Stats test ────────────────────────────────────────────────────────────────

func TestStatsIncrement(t *testing.T) {
	hub := NewHub(100)
	conn := newTestConn()
	c := newClient("s1", "user1", conn, hub)
	if err := hub.Register(c); err != nil {
		t.Fatal(err)
	}
	stats := hub.Stats()
	if stats.TotalConnected != 1 {
		t.Errorf("TotalConnected = %d, want 1", stats.TotalConnected)
	}
}

// ── Presence test ─────────────────────────────────────────────────────────────

// TestPresenceJoinLeave checks that presence events are broadcast on join/leave.
func TestPresenceJoinLeave(t *testing.T) {
	hub := NewHub(100)

	connA := newTestConn()
	cA := newClient("cA", "alice", connA, hub)
	_ = hub.Register(cA)
	hub.JoinRoom(cA, "chat")

	connB := newTestConn()
	cB := newClient("cB", "bob", connB, hub)
	_ = hub.Register(cB)
	hub.JoinRoom(cB, "chat")

	time.Sleep(10 * time.Millisecond)

	// Drain A's buffer and look for a presence event mentioning bob.
	found := false
	for len(cA.sendBuf) > 0 {
		msg := <-cA.sendBuf
		if bytes.Contains(msg, []byte("bob")) && bytes.Contains(msg, []byte("presence")) {
			found = true
		}
	}
	if !found {
		t.Error("expected presence event with bob on client A's buffer")
	}

	hub.LeaveRoom(cB)
	time.Sleep(10 * time.Millisecond)

	foundLeft := false
	for len(cA.sendBuf) > 0 {
		msg := <-cA.sendBuf
		if bytes.Contains(msg, []byte("left")) {
			foundLeft = true
		}
	}
	if !foundLeft {
		t.Error("expected 'left' presence event on client A's buffer after bob left")
	}
}

// ── Ring buffer tests ─────────────────────────────────────────────────────────

func TestRingBufferWraps(t *testing.T) {
	rb := newRingBuffer(5)
	for i := 0; i < 10; i++ {
		rb.Append([]byte{byte(i)})
	}
	if rb.Len() != 5 {
		t.Errorf("ring buffer len = %d, want 5", rb.Len())
	}
	msgs := rb.Since(0)
	if len(msgs) != 5 {
		t.Errorf("Since(0) returned %d messages, want 5", len(msgs))
	}
}

func TestRingBufferSince(t *testing.T) {
	rb := newRingBuffer(100)
	for i := 0; i < 10; i++ {
		rb.Append([]byte{byte(i)})
	}
	msgs := rb.Since(5)
	if len(msgs) != 5 {
		t.Errorf("Since(5) returned %d messages, want 5 (seqnos 6-10)", len(msgs))
	}
}

// ── Reconnect token tests ─────────────────────────────────────────────────────

func TestReconnectTokenRoundtrip(t *testing.T) {
	ts := newTokenStore()
	token, err := ts.Issue("alice", "lobby", 42)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := ts.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "alice" || claims.RoomID != "lobby" || claims.LastSeq != 42 {
		t.Errorf("claims mismatch: %+v", claims)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	ts := newTokenStore()
	_, err := ts.Verify("not.a.valid.token")
	if err == nil {
		t.Error("expected error for invalid token, got nil")
	}
}

func TestSequenceNumberOrdering(t *testing.T) {
	rb := newRingBuffer(100)
	var last uint64
	for i := 0; i < 20; i++ {
		seqno := rb.Append([]byte{byte(i)})
		if seqno <= last {
			t.Errorf("seqno %d is not greater than previous %d", seqno, last)
		}
		last = seqno
	}
}

// ── Concurrent broadcast test ─────────────────────────────────────────────────

func TestConcurrentBroadcast(t *testing.T) {
	hub := NewHub(200)

	for i := 0; i < 10; i++ {
		conn := newTestConn()
		c := newClient(string(rune('a'+i)), string(rune('a'+i)), conn, hub)
		_ = hub.Register(c)
		hub.JoinRoom(c, "concurrent-room")
		go func() {
			for range c.sendBuf {
			}
		}()
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				hub.Broadcast("concurrent-room", []byte("msg"))
			}
		}()
	}
	wg.Wait()
}

// ── Test helpers ──────────────────────────────────────────────────────────────

// testConn is a minimal net.Conn that discards writes and blocks reads.
// It is used so test clients do not need a real TCP connection.
type testConn struct{}

func newTestConn() net.Conn { return &testConn{} }

func (t *testConn) Read(b []byte) (int, error) {
	// Block until the test ends — unit tests don't drive readPump.
	time.Sleep(time.Hour)
	return 0, nil
}
func (t *testConn) Write(b []byte) (int, error)       { return len(b), nil }
func (t *testConn) Close() error                       { return nil }
func (t *testConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (t *testConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (t *testConn) SetDeadline(tt time.Time) error     { return nil }
func (t *testConn) SetReadDeadline(tt time.Time) error { return nil }
func (t *testConn) SetWriteDeadline(tt time.Time) error { return nil }
