// Gossip Protocol HTTP Server
//
// Wraps a SWIM gossip node (v2 — piggybacking) in a simple HTTP API.
// This server is the integration target for the Java Spring Boot client.
//
// Routes:
//
//	GET  /members        — list all known members with status and last-seen time
//	GET  /health         — liveness check: {"status":"ok","aliveCount":N}
//	POST /join           — join the cluster: {"addr":"host:port"}
//	GET  /stats          — gossip metrics: round count, messages sent, member count
//
// The gossip node listens on a UDP port (--gossip-port, default 7946).
// The HTTP API listens on --http-port (default 8080).
//
// Usage:
//
//	go run ./cmd/server --http-port 8080 --gossip-addr 127.0.0.1:7946
//	go run ./cmd/server --http-port 8081 --gossip-addr 127.0.0.1:7947 --join 127.0.0.1:7946
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"dev.pushkar/gossip/pkg/swim"
)

// ── Server state ──────────────────────────────────────────────────────────────

type server struct {
	node *swim.PiggybackNode
}

// ── Request / Response types ──────────────────────────────────────────────────

type memberResponse struct {
	Addr        string `json:"addr"`
	Status      string `json:"status"`
	LastSeen    string `json:"lastSeen"`
	Incarnation uint64 `json:"incarnation"`
}

type healthResponse struct {
	Status     string `json:"status"`
	AliveCount int    `json:"aliveCount"`
	TotalCount int    `json:"totalCount"`
}

type joinRequest struct {
	Addr string `json:"addr"`
}

type statsResponse struct {
	RoundCount   int64  `json:"roundCount"`
	MessagesSent int64  `json:"messagesSent"`
	MemberCount  int    `json:"memberCount"`
	AliveCount   int    `json:"aliveCount"`
	SuspectCount int    `json:"suspectCount"`
	DeadCount    int    `json:"deadCount"`
	SelfAddr     string `json:"selfAddr"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GET /members
func (s *server) members(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	members := s.node.Members()
	resp := make([]memberResponse, 0, len(members))
	for _, m := range members {
		resp = append(resp, memberResponse{
			Addr:        m.Addr,
			Status:      m.Status.String(),
			LastSeen:    m.LastSeen.Format(time.RFC3339),
			Incarnation: m.Incarnation,
		})
	}
	writeJSON(w, resp)
}

// GET /health
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	members := s.node.Members()
	alive := 0
	for _, m := range members {
		if m.Status == swim.StatusAlive {
			alive++
		}
	}
	writeJSON(w, healthResponse{
		Status:     "ok",
		AliveCount: alive,
		TotalCount: len(members),
	})
}

// POST /join  body: {"addr":"127.0.0.1:7947"}
func (s *server) join(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Addr == "" {
		http.Error(w, "addr is required", http.StatusBadRequest)
		return
	}
	s.node.Join(req.Addr)
	writeJSON(w, map[string]string{"status": "joined", "peer": req.Addr})
}

// GET /stats
func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rounds, messages := s.node.Stats()
	members := s.node.Members()
	var alive, suspect, dead int
	for _, m := range members {
		switch m.Status {
		case swim.StatusAlive:
			alive++
		case swim.StatusSuspect:
			suspect++
		case swim.StatusDead:
			dead++
		}
	}
	writeJSON(w, statsResponse{
		RoundCount:   rounds,
		MessagesSent: messages,
		MemberCount:  len(members),
		AliveCount:   alive,
		SuspectCount: suspect,
		DeadCount:    dead,
	})
}

// ── Routing ───────────────────────────────────────────────────────────────────

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/members", s.members)
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/join", s.join)
	mux.HandleFunc("/stats", s.stats)
	return mux
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	httpPort  := flag.Int("http-port", 8080, "HTTP API port")
	gossipAddr := flag.String("gossip-addr", "127.0.0.1:7946", "UDP gossip listen address")
	joinAddr  := flag.String("join", "", "Peer gossip address to join on startup")
	flag.Parse()

	node, err := swim.NewPiggybackNode(*gossipAddr)
	if err != nil {
		log.Fatalf("create gossip node on %s: %v", *gossipAddr, err)
	}

	if *joinAddr != "" {
		node.Join(*joinAddr)
		log.Printf("joining cluster via peer %s", *joinAddr)
	}

	node.Start()
	log.Printf("gossip node started on %s", *gossipAddr)

	srv := &server{node: node}
	addr := fmt.Sprintf(":%d", *httpPort)
	log.Printf("HTTP API starting on %s", addr)
	log.Printf("endpoints: GET /members  GET /health  POST /join  GET /stats")

	if err := http.ListenAndServe(addr, srv.routes()); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}
