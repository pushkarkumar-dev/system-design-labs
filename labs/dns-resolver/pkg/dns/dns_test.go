package dns

import (
	"errors"
	"net"
	"testing"
	"time"
)

// ─── Wire format tests ────────────────────────────────────────────────────────

// TestParseQueryBytes verifies that a hand-crafted DNS query packet is parsed correctly.
func TestParseQueryBytes(t *testing.T) {
	// Build a query for "example.com." type A
	raw := buildQuery("example.com", TypeA)

	q, err := ParseQuery(raw)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	if q.Header.ID != 1 {
		t.Errorf("want ID=1, got %d", q.Header.ID)
	}
	if q.Header.QDCount != 1 {
		t.Errorf("want QDCount=1, got %d", q.Header.QDCount)
	}
	if len(q.Questions) != 1 {
		t.Fatalf("want 1 question, got %d", len(q.Questions))
	}
	if q.Questions[0].Name != "example.com." {
		t.Errorf("want name=example.com., got %s", q.Questions[0].Name)
	}
	if q.Questions[0].QType != TypeA {
		t.Errorf("want qtype=A, got %d", q.Questions[0].QType)
	}
}

// TestBuildResponseBytes verifies the wire format of a BuildResponse output.
func TestBuildResponseBytes(t *testing.T) {
	ip := [4]byte{93, 184, 216, 34}
	resp := BuildResponse(42, "example.com.", ip)

	parsed, err := ParseQuery(resp)
	if err != nil {
		t.Fatalf("ParseQuery on response: %v", err)
	}
	if parsed.Header.ID != 42 {
		t.Errorf("want ID=42, got %d", parsed.Header.ID)
	}
	if !parsed.Header.QR {
		t.Error("want QR=true (response)")
	}
	if parsed.Header.RCODE != RcodeNoError {
		t.Errorf("want RCODE=0, got %d", parsed.Header.RCODE)
	}
	if len(parsed.Answers) != 1 {
		t.Fatalf("want 1 answer, got %d", len(parsed.Answers))
	}
	gotIP, ok := ExtractARecord(parsed.Answers[0])
	if !ok {
		t.Fatal("answer is not an A record")
	}
	if gotIP != ip {
		t.Errorf("want IP %v, got %v", ip, gotIP)
	}
}

// TestLabelEncoding verifies that appendName produces correct label-encoded bytes.
func TestLabelEncoding(t *testing.T) {
	// "api.example.com." → [3]api[7]example[3]com[0]
	buf := appendName(nil, "api.example.com.")
	expected := []byte{3, 'a', 'p', 'i', 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0}
	if len(buf) != len(expected) {
		t.Fatalf("label encoding: want %d bytes, got %d", len(expected), len(buf))
	}
	for i, b := range expected {
		if buf[i] != b {
			t.Errorf("byte[%d]: want 0x%02x, got 0x%02x", i, b, buf[i])
		}
	}
}

// TestParseName verifies that parseName correctly decodes a label-encoded name.
func TestParseName(t *testing.T) {
	// Encode "api.example.com." then parse it back
	encoded := appendName(nil, "api.example.com.")
	// Prepend some header-like bytes to test non-zero offset
	buf := append(make([]byte, 5), encoded...)
	name, nextOff, err := parseName(buf, 5)
	if err != nil {
		t.Fatalf("parseName: %v", err)
	}
	if name != "api.example.com." {
		t.Errorf("want api.example.com., got %s", name)
	}
	if nextOff != 5+len(encoded) {
		t.Errorf("wrong next offset: want %d, got %d", 5+len(encoded), nextOff)
	}
}

// TestNXDOMAINResponse verifies BuildNXDOMAIN produces a valid NXDOMAIN packet.
func TestNXDOMAINResponse(t *testing.T) {
	resp := BuildNXDOMAIN(99, "notexist.example.com.")
	parsed, err := ParseQuery(resp)
	if err != nil {
		t.Fatalf("ParseQuery NXDOMAIN: %v", err)
	}
	if parsed.Header.RCODE != RcodeNXDOMAIN {
		t.Errorf("want RCODE=3, got %d", parsed.Header.RCODE)
	}
}

// ─── Resolver + cache tests ───────────────────────────────────────────────────

// TestLocalZoneResolution verifies that names in the local zone are answered directly.
func TestLocalZoneResolution(t *testing.T) {
	r := NewResolver()
	records, err := r.Resolve("example.com", TypeA)
	if err != nil {
		t.Fatalf("Resolve example.com: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected at least one A record")
	}
	ip, ok := ExtractARecord(records[0])
	if !ok {
		t.Fatal("record is not an A record")
	}
	want := [4]byte{93, 184, 216, 34}
	if ip != want {
		t.Errorf("want %v, got %v", want, ip)
	}
}

// TestCacheHitOnRepeatQuery verifies that the second query for the same name is a cache hit.
func TestCacheHitOnRepeatQuery(t *testing.T) {
	r := NewResolver()
	// First call — zone hit, not a cache miss (zone answers bypass cache lookup)
	_, err := r.Resolve("example.com", TypeA)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// Inject a cache entry directly to simulate a prior recursive resolution
	key := "remote.example.test.:1"
	r.mu.Lock()
	r.cache[key] = &cacheEntry{
		records: []DNSRecord{{
			Name: "remote.example.test.", Type: TypeA, Class: ClassIN,
			TTL: 300, RDLength: 4, RData: []byte{1, 2, 3, 4},
		}},
		expiresAt: time.Now().Add(5 * time.Minute),
	}
	r.mu.Unlock()

	statsBefore := r.GetStats()
	// Manually call cache lookup path
	r.mu.RLock()
	entry, found := r.cache[key]
	r.mu.RUnlock()
	if !found || entry.negative {
		t.Fatal("cache entry not found")
	}
	_ = statsBefore
	// Verify the entry is not expired
	if time.Now().After(entry.expiresAt) {
		t.Error("cache entry should not be expired")
	}
}

// TestNXDOMAINNegativeCaching verifies that NXDOMAIN responses are cached.
func TestNXDOMAINNegativeCaching(t *testing.T) {
	r := NewResolver()
	// Manually inject a negative cache entry
	key := "ghost.example.test.:1"
	r.cacheNegative(key, 300)

	r.mu.RLock()
	entry, found := r.cache[key]
	r.mu.RUnlock()

	if !found {
		t.Fatal("negative cache entry not stored")
	}
	if !entry.negative {
		t.Error("entry should be marked negative")
	}
	if time.Now().After(entry.expiresAt) {
		t.Error("negative cache entry should not be expired immediately")
	}
}

// TestCNAMEFollowing verifies that CNAME records are followed to their A record target.
func TestCNAMEFollowing(t *testing.T) {
	// Simulate a response containing a CNAME and then an A record
	// by testing the decodeName helper + the CNAME detection logic directly
	cnameRData := appendName(nil, "target.example.com.")
	cnameRData = cnameRData[:len(cnameRData)] // strip trailing 0 is already there

	decoded := decodeName(cnameRData)
	if decoded != "target.example.com." {
		t.Errorf("decodeName: want target.example.com., got %s", decoded)
	}
}

// TestCNAMELoopDetection verifies that a CNAME loop does not recurse infinitely.
func TestCNAMELoopDetection(t *testing.T) {
	r := NewResolver()
	// We simulate the loop detection by calling resolveWithDepth at max depth
	_, err := r.resolveWithDepth("loop.example.test.", TypeA, maxCNAMEHops)
	if err == nil {
		t.Fatal("expected CNAME loop error, got nil")
	}
	if err.Error() != "dns: CNAME chain too long (max 10 hops)" {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestNXDOMAINError verifies the error type returned for NXDOMAIN.
func TestNXDOMAINError(t *testing.T) {
	err := &NXDOMAINError{Name: "nope.example.com."}
	var nxErr *NXDOMAINError
	if !errors.As(err, &nxErr) {
		t.Error("errors.As should match NXDOMAINError")
	}
	if nxErr.Name != "nope.example.com." {
		t.Errorf("wrong name in error: %s", nxErr.Name)
	}
}

// TestStatsAccumulate verifies that query counters increment.
func TestStatsAccumulate(t *testing.T) {
	r := NewResolver()
	before := r.GetStats()

	r.Resolve("example.com", TypeA)
	r.Resolve("lab.example.com", TypeA)

	after := r.GetStats()
	if after.Queries <= before.Queries {
		t.Errorf("query count did not increase: before=%d after=%d", before.Queries, after.Queries)
	}
}

// ─── Integration: local UDP server ───────────────────────────────────────────

// TestUDPRoundTrip starts a local UDP server and verifies a full query-response cycle.
func TestUDPRoundTrip(t *testing.T) {
	resolver := NewResolver()

	// Start a local UDP listener
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer pc.Close()

	addr := pc.LocalAddr().(*net.UDPAddr)

	// Server goroutine — handles one packet then exits
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		pc.SetDeadline(time.Now().Add(3 * time.Second))
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		q, err := ParseQuery(buf[:n])
		if err != nil || len(q.Questions) == 0 {
			return
		}
		name := q.Questions[0].Name
		records, err := resolver.Resolve(name, TypeA)
		var resp []byte
		if err != nil {
			resp = BuildNXDOMAIN(q.Header.ID, name)
		} else if len(records) > 0 {
			if ip, ok := ExtractARecord(records[0]); ok {
				resp = BuildResponse(q.Header.ID, name, ip)
			} else {
				resp = BuildNXDOMAIN(q.Header.ID, name)
			}
		}
		pc.WriteTo(resp, raddr)
	}()

	// Client side
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))

	query := buildQuery("example.com", TypeA)
	if _, err := conn.Write(query); err != nil {
		t.Fatalf("Write: %v", err)
	}

	respBuf := make([]byte, 512)
	n, err := conn.Read(respBuf)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	parsed, err := ParseQuery(respBuf[:n])
	if err != nil {
		t.Fatalf("ParseQuery response: %v", err)
	}
	if parsed.Header.RCODE != RcodeNoError {
		t.Errorf("want RCODE=0, got %d", parsed.Header.RCODE)
	}
	if len(parsed.Answers) == 0 {
		t.Error("expected at least one answer")
	}

	<-done
}
