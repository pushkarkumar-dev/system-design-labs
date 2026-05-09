package dns_test

import (
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/dns-resolver/pkg/dns"
)

// BenchmarkParseQuery measures raw wire-format parsing throughput.
// Estimated: ~2,000,000 packets/sec on M2 MacBook Pro.
func BenchmarkParseQuery(b *testing.B) {
	pkt := buildBenchPacket("example.com", dns.TypeA)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = dns.ParseQuery(pkt)
	}
}

// BenchmarkBuildResponse measures DNS response construction throughput.
func BenchmarkBuildResponse(b *testing.B) {
	ip := [4]byte{93, 184, 216, 34}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dns.BuildResponse(uint16(i), "example.com.", ip)
	}
}

// BenchmarkCacheHit measures resolver throughput for zone-hit queries (simulates warm cache).
// Estimated: ~450,000 queries/sec on M2 MacBook Pro.
func BenchmarkCacheHit(b *testing.B) {
	r := dns.NewResolver()
	_, _ = r.Resolve("example.com", dns.TypeA)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.Resolve("example.com", dns.TypeA)
	}
}

// buildBenchPacket builds a raw DNS query packet for benchmarks.
func buildBenchPacket(name string, qtype uint16) []byte {
	pkt := make([]byte, 0, 64)
	pkt = append(pkt, 0, 1)                    // ID = 1
	pkt = append(pkt, 0x01, 0x00)              // Flags: RD=1
	pkt = append(pkt, 0, 1, 0, 0, 0, 0, 0, 0) // QDCOUNT=1

	n := name
	for {
		dot := -1
		for i, c := range n {
			if c == '.' {
				dot = i
				break
			}
		}
		if dot == -1 {
			pkt = append(pkt, byte(len(n)))
			pkt = append(pkt, []byte(n)...)
			break
		}
		label := n[:dot]
		pkt = append(pkt, byte(len(label)))
		pkt = append(pkt, []byte(label)...)
		n = n[dot+1:]
	}
	pkt = append(pkt, 0)
	pkt = append(pkt, byte(qtype>>8), byte(qtype), 0, 1)
	return pkt
}
