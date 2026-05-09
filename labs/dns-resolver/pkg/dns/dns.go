// Package dns implements a toy recursive DNS resolver.
//
// v0: DNS wire-format parsing + local zone file + UDP server
// v1: Recursive resolution — follows delegation chain root → TLD → auth
// v2: Negative caching (NXDOMAIN) + CNAME following with loop detection
package dns

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// ─── Wire-format constants ────────────────────────────────────────────────────

const (
	TypeA     = 1
	TypeNS    = 2
	TypeCNAME = 5
	TypeSOA   = 6
	TypeAAAA  = 28

	ClassIN = 1

	// RcodeNXDOMAIN is the DNS response code for "domain does not exist".
	RcodeNXDOMAIN = 3
	// RcodeNoError is the DNS response code for success.
	RcodeNoError = 0

	maxCNAMEHops = 10

	// headerLen is the fixed DNS header size in bytes.
	headerLen = 12
)

// Root nameserver — one real anycast IP (198.41.0.4 = a.root-servers.net).
// In tests this is overridden via RootServers.
var defaultRootServers = []string{"198.41.0.4:53"}

// ─── Types ────────────────────────────────────────────────────────────────────

// DNSHeader represents a parsed DNS message header.
type DNSHeader struct {
	ID      uint16
	QR      bool   // true = response, false = query
	Opcode  uint8  // 0 = QUERY
	AA      bool   // authoritative answer
	TC      bool   // truncated
	RD      bool   // recursion desired
	RA      bool   // recursion available
	Z       uint8  // reserved
	RCODE   uint8  // response code
	QDCount uint16 // question count
	ANCount uint16 // answer count
	NSCount uint16 // authority count
	ARCount uint16 // additional count
}

// DNSQuestion is a single entry in the question section.
type DNSQuestion struct {
	Name   string
	QType  uint16
	QClass uint16
}

// DNSRecord represents a resource record.
type DNSRecord struct {
	Name     string
	Type     uint16
	Class    uint16
	TTL      uint32
	RDLength uint16
	RData    []byte
}

// DNSQuery is the fully parsed DNS request.
type DNSQuery struct {
	Header    DNSHeader
	Questions []DNSQuestion
	Answers   []DNSRecord
	Authority []DNSRecord
	Additional []DNSRecord
}

// ─── Cache ────────────────────────────────────────────────────────────────────

// cacheEntry holds one cached DNS answer.
type cacheEntry struct {
	records   []DNSRecord
	negative  bool // true = NXDOMAIN cached
	expiresAt time.Time
}

// GetNegative returns true if this is a negative (NXDOMAIN) cache entry.
func (e *cacheEntry) GetNegative() bool { return e.negative }

// GetExpiresAt returns the expiry time of this cache entry.
func (e *cacheEntry) GetExpiresAt() time.Time { return e.expiresAt }

// RecordCount returns the number of cached records (0 for negative entries).
func (e *cacheEntry) RecordCount() int { return len(e.records) }

// Stats holds runtime counters.
type Stats struct {
	Queries     uint64
	CacheHits   uint64
	CacheMisses uint64
	NXDOMAINs   uint64
}

// Resolver is the top-level DNS resolver with cache and optional root-server override.
type Resolver struct {
	mu          sync.RWMutex
	cache       map[string]*cacheEntry // key = "name:type"
	zone        map[string][4]byte     // local authoritative zone
	RootServers []string               // overridable for tests
	stats       Stats
}

// NewResolver creates a new resolver with a hardcoded local zone.
func NewResolver() *Resolver {
	r := &Resolver{
		cache:       make(map[string]*cacheEntry),
		zone:        make(map[string][4]byte),
		RootServers: defaultRootServers,
	}
	// Hardcoded zone — the resolver answers these authoritatively.
	r.zone["example.com."] = [4]byte{93, 184, 216, 34}
	r.zone["lab.example.com."] = [4]byte{10, 0, 0, 1}
	r.zone["alias.example.com."] = [4]byte{10, 0, 0, 2}
	return r
}

// GetStats returns a snapshot of the resolver statistics.
func (r *Resolver) GetStats() Stats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.stats
}

// GetCacheEntries returns a copy of all live cache entries for the admin API.
func (r *Resolver) GetCacheEntries() map[string]*cacheEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	out := make(map[string]*cacheEntry)
	for k, v := range r.cache {
		if v.expiresAt.After(now) {
			out[k] = v
		}
	}
	return out
}

// FlushCache removes all cached entries.
func (r *Resolver) FlushCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*cacheEntry)
}

// ─── Wire format: parsing ─────────────────────────────────────────────────────

// ParseQuery parses raw DNS wire-format bytes into a DNSQuery.
// Returns an error if the packet is too short or malformed.
func ParseQuery(buf []byte) (*DNSQuery, error) {
	if len(buf) < headerLen {
		return nil, errors.New("dns: packet too short for header")
	}

	q := &DNSQuery{}
	flags := binary.BigEndian.Uint16(buf[2:4])

	q.Header = DNSHeader{
		ID:      binary.BigEndian.Uint16(buf[0:2]),
		QR:      flags>>15 == 1,
		Opcode:  uint8((flags >> 11) & 0xF),
		AA:      (flags>>10)&1 == 1,
		TC:      (flags>>9)&1 == 1,
		RD:      (flags>>8)&1 == 1,
		RA:      (flags>>7)&1 == 1,
		RCODE:   uint8(flags & 0xF),
		QDCount: binary.BigEndian.Uint16(buf[4:6]),
		ANCount: binary.BigEndian.Uint16(buf[6:8]),
		NSCount: binary.BigEndian.Uint16(buf[8:10]),
		ARCount: binary.BigEndian.Uint16(buf[10:12]),
	}

	offset := headerLen

	// Parse questions
	for i := 0; i < int(q.Header.QDCount); i++ {
		name, newOffset, err := parseName(buf, offset)
		if err != nil {
			return nil, fmt.Errorf("dns: question name parse: %w", err)
		}
		offset = newOffset
		if offset+4 > len(buf) {
			return nil, errors.New("dns: question section truncated")
		}
		qtype := binary.BigEndian.Uint16(buf[offset : offset+2])
		qclass := binary.BigEndian.Uint16(buf[offset+2 : offset+4])
		offset += 4
		q.Questions = append(q.Questions, DNSQuestion{Name: name, QType: qtype, QClass: qclass})
	}

	// Parse answer, authority, additional sections
	q.Answers, offset = parseRRSection(buf, offset, int(q.Header.ANCount))
	q.Authority, offset = parseRRSection(buf, offset, int(q.Header.NSCount))
	q.Additional, _ = parseRRSection(buf, offset, int(q.Header.ARCount))

	return q, nil
}

func parseRRSection(buf []byte, offset, count int) ([]DNSRecord, int) {
	records := make([]DNSRecord, 0, count)
	for i := 0; i < count; i++ {
		rec, newOffset, err := parseRecord(buf, offset)
		if err != nil {
			break
		}
		records = append(records, rec)
		offset = newOffset
	}
	return records, offset
}

// parseRecord parses one resource record starting at offset.
func parseRecord(buf []byte, offset int) (DNSRecord, int, error) {
	name, newOffset, err := parseName(buf, offset)
	if err != nil {
		return DNSRecord{}, offset, err
	}
	offset = newOffset
	if offset+10 > len(buf) {
		return DNSRecord{}, offset, errors.New("dns: RR too short")
	}
	rec := DNSRecord{
		Name:     name,
		Type:     binary.BigEndian.Uint16(buf[offset : offset+2]),
		Class:    binary.BigEndian.Uint16(buf[offset+2 : offset+4]),
		TTL:      binary.BigEndian.Uint32(buf[offset+4 : offset+8]),
		RDLength: binary.BigEndian.Uint16(buf[offset+8 : offset+10]),
	}
	offset += 10
	if offset+int(rec.RDLength) > len(buf) {
		return DNSRecord{}, offset, errors.New("dns: RDATA truncated")
	}
	rec.RData = buf[offset : offset+int(rec.RDLength)]
	offset += int(rec.RDLength)
	return rec, offset, nil
}

// parseName decodes a DNS name (label encoding) starting at offset.
// Handles compression pointers (RFC 1035 §4.1.4).
func parseName(buf []byte, offset int) (string, int, error) {
	var name []byte
	visited := make(map[int]bool)
	followedPointer := false
	nextOffset := offset

	for {
		if offset >= len(buf) {
			return "", 0, errors.New("dns: name extends beyond packet")
		}
		if visited[offset] {
			return "", 0, errors.New("dns: compression loop detected")
		}
		visited[offset] = true

		labelLen := int(buf[offset])
		if labelLen == 0 {
			// root label — end of name
			if !followedPointer {
				nextOffset = offset + 1
			}
			break
		}

		// Compression pointer: top two bits are 11
		if labelLen&0xC0 == 0xC0 {
			if offset+1 >= len(buf) {
				return "", 0, errors.New("dns: compression pointer truncated")
			}
			ptr := int(binary.BigEndian.Uint16(buf[offset:offset+2]) & 0x3FFF)
			if !followedPointer {
				nextOffset = offset + 2
			}
			followedPointer = true
			offset = ptr
			continue
		}

		offset++
		if offset+labelLen > len(buf) {
			return "", 0, errors.New("dns: label extends beyond packet")
		}
		if len(name) > 0 {
			name = append(name, '.')
		}
		name = append(name, buf[offset:offset+labelLen]...)
		offset += labelLen
	}

	result := string(name) + "."
	return result, nextOffset, nil
}

// ─── Wire format: building ────────────────────────────────────────────────────

// BuildResponse constructs a DNS A-record response for the given query ID, name, and IP.
func BuildResponse(id uint16, name string, ip [4]byte) []byte {
	buf := make([]byte, 0, 512)

	// Header: ID
	buf = append(buf, byte(id>>8), byte(id))
	// Flags: QR=1 (response), AA=1, RD=1, RA=1, RCODE=0
	flags := uint16(0x8580) // QR=1, Opcode=0, AA=1, RD=1, RA=1
	buf = append(buf, byte(flags>>8), byte(flags))
	// QDCOUNT=1, ANCOUNT=1, NSCOUNT=0, ARCOUNT=0
	buf = append(buf, 0, 1, 0, 1, 0, 0, 0, 0)

	// Question section
	buf = appendName(buf, name)
	buf = append(buf, 0, TypeA, 0, ClassIN)

	// Answer section
	buf = appendName(buf, name)
	buf = append(buf, 0, TypeA, 0, ClassIN)
	// TTL = 300
	buf = append(buf, 0, 0, 1, 44)
	// RDLENGTH = 4
	buf = append(buf, 0, 4)
	buf = append(buf, ip[0], ip[1], ip[2], ip[3])

	return buf
}

// BuildNXDOMAIN constructs a DNS NXDOMAIN response.
func BuildNXDOMAIN(id uint16, name string) []byte {
	buf := make([]byte, 0, 64)
	buf = append(buf, byte(id>>8), byte(id))
	// Flags: QR=1, AA=1, RD=1, RA=1, RCODE=3 (NXDOMAIN)
	flags := uint16(0x8583)
	buf = append(buf, byte(flags>>8), byte(flags))
	// QDCOUNT=1, ANCOUNT=0, NSCOUNT=0, ARCOUNT=0
	buf = append(buf, 0, 1, 0, 0, 0, 0, 0, 0)
	buf = appendName(buf, name)
	buf = append(buf, 0, TypeA, 0, ClassIN)
	return buf
}

// BuildErrorResponse builds a SERVFAIL or other error response.
func BuildErrorResponse(id uint16, name string, rcode uint8) []byte {
	buf := make([]byte, 0, 64)
	buf = append(buf, byte(id>>8), byte(id))
	flags := uint16(0x8500) | uint16(rcode)
	buf = append(buf, byte(flags>>8), byte(flags))
	buf = append(buf, 0, 1, 0, 0, 0, 0, 0, 0)
	buf = appendName(buf, name)
	buf = append(buf, 0, TypeA, 0, ClassIN)
	return buf
}

// appendName encodes a DNS name as length-prefixed labels.
func appendName(buf []byte, name string) []byte {
	// Strip trailing dot if present for label splitting
	n := name
	if len(n) > 0 && n[len(n)-1] == '.' {
		n = n[:len(n)-1]
	}
	if n == "" {
		return append(buf, 0)
	}
	start := 0
	for i := 0; i <= len(n); i++ {
		if i == len(n) || n[i] == '.' {
			label := n[start:i]
			buf = append(buf, byte(len(label)))
			buf = append(buf, []byte(label)...)
			start = i + 1
		}
	}
	buf = append(buf, 0) // root label
	return buf
}

// ExtractARecord reads the first A-record IP from RDATA.
func ExtractARecord(rec DNSRecord) ([4]byte, bool) {
	if rec.Type == TypeA && len(rec.RData) == 4 {
		return [4]byte{rec.RData[0], rec.RData[1], rec.RData[2], rec.RData[3]}, true
	}
	return [4]byte{}, false
}

// ExtractName reads a DNS name from an RR's RDATA (used for NS and CNAME records).
func ExtractName(buf []byte, rdata []byte, rdataOffset int) (string, bool) {
	// rdata is a slice of the full buf; we need the full buf for pointer resolution.
	// We store the full packet buffer alongside parsed records via parsedMsg.
	name, _, err := parseName(buf, rdataOffset)
	if err != nil {
		return "", false
	}
	return name, true
}

// ─── Recursive resolver (v1) ──────────────────────────────────────────────────

// Resolve performs a full recursive resolution for name/qtype.
// It first checks the local zone, then the TTL cache, then recurses from root.
func (r *Resolver) Resolve(name string, qtype uint16) ([]DNSRecord, error) {
	r.mu.Lock()
	r.stats.Queries++
	r.mu.Unlock()

	// Normalise to FQDN
	if len(name) == 0 || name[len(name)-1] != '.' {
		name = name + "."
	}

	return r.resolveWithDepth(name, qtype, 0)
}

func (r *Resolver) resolveWithDepth(name string, qtype uint16, depth int) ([]DNSRecord, error) {
	if depth > maxCNAMEHops {
		return nil, errors.New("dns: CNAME chain too long (max 10 hops)")
	}

	// 1. Local zone — answer authoritatively
	if qtype == TypeA {
		if ip, ok := r.zone[name]; ok {
			return []DNSRecord{{
				Name:     name,
				Type:     TypeA,
				Class:    ClassIN,
				TTL:      300,
				RDLength: 4,
				RData:    ip[:],
			}}, nil
		}
	}

	// 2. Cache lookup
	cacheKey := fmt.Sprintf("%s:%d", name, qtype)
	r.mu.RLock()
	entry, found := r.cache[cacheKey]
	r.mu.RUnlock()

	if found && time.Now().Before(entry.expiresAt) {
		r.mu.Lock()
		r.stats.CacheHits++
		r.mu.Unlock()
		if entry.negative {
			return nil, &NXDOMAINError{Name: name}
		}
		return entry.records, nil
	}

	r.mu.Lock()
	r.stats.CacheMisses++
	r.mu.Unlock()

	// 3. Recursive resolution — walk the delegation tree
	records, err := r.recurse(name, qtype, r.RootServers)
	if err != nil {
		var nxErr *NXDOMAINError
		if errors.As(err, &nxErr) {
			// Cache the NXDOMAIN with a 5-minute negative TTL
			r.cacheNegative(cacheKey, 300)
		}
		return nil, err
	}

	// 4. Handle CNAME: if answer is a CNAME, follow it
	for _, rec := range records {
		if rec.Type == TypeCNAME {
			target := decodeName(rec.RData)
			if target == "" {
				continue
			}
			// Cache the CNAME itself
			r.cacheRecords(cacheKey, records, rec.TTL)
			// Follow the CNAME target
			targetRecords, err := r.resolveWithDepth(target, qtype, depth+1)
			if err != nil {
				return records, nil // return the CNAME at least
			}
			return append(records, targetRecords...), nil
		}
	}

	// Cache positive answer
	ttl := minTTL(records)
	r.cacheRecords(cacheKey, records, ttl)

	return records, nil
}

// recurse walks the delegation chain starting from the given nameservers.
func (r *Resolver) recurse(name string, qtype uint16, nameservers []string) ([]DNSRecord, error) {
	servers := nameservers

	for round := 0; round < 16; round++ {
		if len(servers) == 0 {
			return nil, fmt.Errorf("dns: no nameservers available for %s", name)
		}

		// Try each nameserver
		var lastErr error
		for _, ns := range servers {
			resp, rawBuf, err := queryNameserver(ns, name, qtype)
			if err != nil {
				lastErr = err
				continue
			}

			// Check for NXDOMAIN
			if resp.Header.RCODE == RcodeNXDOMAIN {
				return nil, &NXDOMAINError{Name: name}
			}

			// Got answers — return them
			if len(resp.Answers) > 0 {
				// Check if answers are for our qtype or a CNAME
				for _, ans := range resp.Answers {
					if ans.Type == qtype || ans.Type == TypeCNAME {
						return resp.Answers, nil
					}
				}
			}

			// Got authority section — follow NS delegation
			if len(resp.Authority) > 0 {
				nextServers := extractNSAddresses(resp, rawBuf)
				if len(nextServers) > 0 {
					servers = nextServers
					lastErr = nil
					break
				}
				// NS records without glue — resolve NS names separately
				nextServers, err = r.resolveNSNames(resp.Authority, rawBuf)
				if err == nil && len(nextServers) > 0 {
					servers = nextServers
					lastErr = nil
					break
				}
			}

			lastErr = fmt.Errorf("dns: no useful answer from %s", ns)
		}

		if lastErr != nil {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("dns: max delegation depth reached for %s", name)
}

// queryNameserver sends a DNS query to addr and returns the parsed response + raw bytes.
func queryNameserver(addr, name string, qtype uint16) (*DNSQuery, []byte, error) {
	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	query := buildQuery(name, qtype)
	if _, err := conn.Write(query); err != nil {
		return nil, nil, err
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, nil, err
	}
	raw := buf[:n]

	resp, err := ParseQuery(raw)
	if err != nil {
		return nil, nil, err
	}
	return resp, raw, nil
}

// buildQuery constructs a DNS query packet.
func buildQuery(name string, qtype uint16) []byte {
	buf := make([]byte, 0, 64)
	// Header
	buf = append(buf, 0, 1) // ID = 1
	// Flags: RD=1
	buf = append(buf, 0x01, 0x00)
	// QDCOUNT=1, AN/NS/AR=0
	buf = append(buf, 0, 1, 0, 0, 0, 0, 0, 0)
	buf = appendName(buf, name)
	buf = append(buf, byte(qtype>>8), byte(qtype), 0, ClassIN)
	return buf
}

// extractNSAddresses extracts nameserver IP:port strings from the additional section (glue records).
// Toy simplification: returns all A records from Additional without filtering by NS name.
// A production resolver would cross-reference NS names from Authority against Additional names.
func extractNSAddresses(resp *DNSQuery, _ []byte) []string {
	var addrs []string
	for _, add := range resp.Additional {
		if add.Type == TypeA && len(add.RData) == 4 {
			addrs = append(addrs, fmt.Sprintf("%d.%d.%d.%d:53",
				add.RData[0], add.RData[1], add.RData[2], add.RData[3]))
		}
	}
	return addrs
}

// resolveNSNames resolves NS record names to IP addresses for nameservers without glue.
func (r *Resolver) resolveNSNames(authority []DNSRecord, rawBuf []byte) ([]string, error) {
	var addrs []string
	for _, auth := range authority {
		if auth.Type != TypeNS {
			continue
		}
		nsName, _, err := parseName(rawBuf, findRDataOffset(rawBuf, auth))
		if err != nil {
			continue
		}
		recs, err := r.Resolve(nsName, TypeA)
		if err != nil {
			continue
		}
		for _, rec := range recs {
			if rec.Type == TypeA && len(rec.RData) == 4 {
				addrs = append(addrs, fmt.Sprintf("%d.%d.%d.%d:53",
					rec.RData[0], rec.RData[1], rec.RData[2], rec.RData[3]))
			}
		}
	}
	if len(addrs) == 0 {
		return nil, errors.New("dns: could not resolve any NS names")
	}
	return addrs, nil
}

// findRDataOffset finds the byte offset of a record's RDATA in the raw packet.
// This is a linear scan — acceptable for our toy size.
func findRDataOffset(buf []byte, rec DNSRecord) int {
	// Re-parse from headerLen to find the record's RDATA offset.
	offset := headerLen
	if len(buf) < headerLen {
		return 0
	}
	qdcount := int(binary.BigEndian.Uint16(buf[4:6]))
	// Skip questions
	for i := 0; i < qdcount; i++ {
		_, newOff, err := parseName(buf, offset)
		if err != nil {
			return 0
		}
		offset = newOff + 4
	}
	// Scan through records to find this one
	totalRRs := int(binary.BigEndian.Uint16(buf[6:8])) +
		int(binary.BigEndian.Uint16(buf[8:10])) +
		int(binary.BigEndian.Uint16(buf[10:12]))
	for i := 0; i < totalRRs; i++ {
		_, newOff, err := parseName(buf, offset)
		if err != nil {
			return 0
		}
		offset = newOff
		if offset+10 > len(buf) {
			return 0
		}
		rtype := binary.BigEndian.Uint16(buf[offset : offset+2])
		rdlen := binary.BigEndian.Uint16(buf[offset+8 : offset+10])
		offset += 10
		if rtype == rec.Type {
			return offset
		}
		offset += int(rdlen)
	}
	return 0
}

// ─── Cache helpers ────────────────────────────────────────────────────────────

func (r *Resolver) cacheRecords(key string, records []DNSRecord, ttl uint32) {
	if ttl == 0 {
		ttl = 60
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = &cacheEntry{
		records:   records,
		negative:  false,
		expiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}
}

func (r *Resolver) cacheNegative(key string, ttl uint32) {
	if ttl == 0 {
		ttl = 300
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = &cacheEntry{
		negative:  true,
		expiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}
	r.stats.NXDOMAINs++
}

func minTTL(records []DNSRecord) uint32 {
	if len(records) == 0 {
		return 60
	}
	min := records[0].TTL
	for _, r := range records[1:] {
		if r.TTL < min {
			min = r.TTL
		}
	}
	return min
}

// decodeName decodes a DNS name from RDATA bytes (no compression pointers).
func decodeName(rdata []byte) string {
	var name []byte
	i := 0
	for i < len(rdata) {
		labelLen := int(rdata[i])
		if labelLen == 0 {
			break
		}
		i++
		if i+labelLen > len(rdata) {
			break
		}
		if len(name) > 0 {
			name = append(name, '.')
		}
		name = append(name, rdata[i:i+labelLen]...)
		i += labelLen
	}
	if len(name) == 0 {
		return ""
	}
	return string(name) + "."
}

// ─── Errors ───────────────────────────────────────────────────────────────────

// NXDOMAINError is returned when a domain definitively does not exist.
type NXDOMAINError struct {
	Name string
}

func (e *NXDOMAINError) Error() string {
	return fmt.Sprintf("dns: NXDOMAIN: %s does not exist", e.Name)
}
