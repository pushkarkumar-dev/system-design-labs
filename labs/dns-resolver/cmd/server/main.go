// DNS resolver server — UDP port 5300 + HTTP admin API.
//
// UDP server:  DNS queries on port 5300 (use dig @127.0.0.1 -p 5300 example.com)
// HTTP admin:
//   GET    /health         → {"status":"ok"}
//   GET    /stats          → query counters
//   GET    /cache          → all live cache entries
//   DELETE /cache          → flush cache
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/dns-resolver/pkg/dns"
)

var (
	udpAddr   = flag.String("udp", "0.0.0.0:5300", "UDP DNS listen address")
	adminAddr = flag.String("admin", "0.0.0.0:5380", "HTTP admin listen address")
)

func main() {
	flag.Parse()

	resolver := dns.NewResolver()

	// Start UDP DNS server
	go func() {
		if err := runUDPServer(*udpAddr, resolver); err != nil {
			log.Fatalf("UDP server: %v", err)
		}
	}()

	// Start HTTP admin server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := resolver.GetStats()
		json.NewEncoder(w).Encode(stats)
	})
	mux.HandleFunc("GET /cache", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entries := resolver.GetCacheEntries()
		type entry struct {
			Key      string    `json:"key"`
			Negative bool      `json:"negative"`
			Expires  time.Time `json:"expires_at"`
			Records  int       `json:"record_count"`
		}
		out := make([]entry, 0, len(entries))
		for k, v := range entries {
			out = append(out, entry{
				Key:      k,
				Negative: v.GetNegative(),
				Expires:  v.GetExpiresAt(),
				Records:  v.RecordCount(),
			})
		}
		json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("DELETE /cache", func(w http.ResponseWriter, r *http.Request) {
		resolver.FlushCache()
		w.WriteHeader(http.StatusNoContent)
	})

	log.Printf("DNS resolver listening on UDP %s", *udpAddr)
	log.Printf("HTTP admin API on %s", *adminAddr)
	log.Printf("Try: dig @127.0.0.1 -p 5300 example.com")

	if err := http.ListenAndServe(*adminAddr, mux); err != nil {
		log.Fatalf("admin HTTP: %v", err)
	}
}

// runUDPServer listens for DNS queries on UDP and responds using the resolver.
func runUDPServer(addr string, resolver *dns.Resolver) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("ListenPacket %s: %w", addr, err)
	}
	defer pc.Close()

	buf := make([]byte, 512)
	for {
		n, raddr, err := pc.ReadFrom(buf)
		if err != nil {
			log.Printf("UDP read error: %v", err)
			continue
		}
		// Handle each query in a goroutine so slow upstream resolutions don't block others
		packet := make([]byte, n)
		copy(packet, buf[:n])
		go handleQuery(pc, raddr, packet, resolver)
	}
}

func handleQuery(pc net.PacketConn, raddr net.Addr, packet []byte, resolver *dns.Resolver) {
	q, err := dns.ParseQuery(packet)
	if err != nil {
		log.Printf("parse error from %s: %v", raddr, err)
		return
	}
	if len(q.Questions) == 0 {
		return
	}

	question := q.Questions[0]
	log.Printf("query from %s: %s type %d", raddr, question.Name, question.QType)

	records, err := resolver.Resolve(question.Name, question.QType)

	var resp []byte
	if err != nil {
		// NXDOMAIN or other error
		resp = dns.BuildNXDOMAIN(q.Header.ID, question.Name)
	} else if len(records) == 0 {
		resp = dns.BuildNXDOMAIN(q.Header.ID, question.Name)
	} else {
		// Find the first A record in the answer set
		for _, rec := range records {
			if ip, ok := dns.ExtractARecord(rec); ok {
				resp = dns.BuildResponse(q.Header.ID, question.Name, ip)
				break
			}
		}
		if resp == nil {
			// Only CNAME records — still return NXDOMAIN for simplicity
			resp = dns.BuildNXDOMAIN(q.Header.ID, question.Name)
		}
	}

	if _, err := pc.WriteTo(resp, raddr); err != nil {
		log.Printf("write error to %s: %v", raddr, err)
	}
}
