// Hash Ring HTTP Server
//
// Exposes the consistent hashing ring over a simple REST API so any language
// can use it for routing decisions without reimplementing the ring.
//
// Routes:
//
//	POST   /nodes          — add a node: {"name":"cache-1","addr":"10.0.0.1:6379"}
//	DELETE /nodes/{name}   — remove a node by name
//	GET    /route?key=...  — returns {"node":"cache-1","addr":"10.0.0.1:6379"}
//	GET    /stats          — distribution stats (key count per node, std deviation)
//	GET    /health         — liveness check
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"

	"dev.pushkar/consistent-hashing/pkg/ring"
)

// ── Server state ──────────────────────────────────────────────────────────────

type server struct {
	ring    *ring.ManagedRing
	mu      sync.Mutex
	tracked []string // sample keys for remap stat reporting
}

// ── Request / Response types ──────────────────────────────────────────────────

type addNodeRequest struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

type routeResponse struct {
	Node string `json:"node"`
	Addr string `json:"addr"`
}

type statsResponse struct {
	Nodes       int                `json:"nodes"`
	KeysPerNode map[string]int     `json:"keysPerNode"`
	StdDev      float64            `json:"stdDevCV"`   // coefficient of variation
	Min         int                `json:"min"`
	Max         int                `json:"max"`
}

type healthResponse struct {
	Status string `json:"status"`
	Nodes  int    `json:"nodes"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// POST /nodes
func (s *server) addNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req addNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Addr == "" {
		http.Error(w, "name and addr are required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.ring.AddNode(ring.Node{Name: req.Name, Addr: req.Addr})
	s.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"status": "added", "name": req.Name})
}

// DELETE /nodes/{name}
func (s *server) removeNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract name from path: /nodes/{name}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/nodes/"), "/")
	name := parts[0]
	if name == "" {
		http.Error(w, "node name required in path", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.ring.RemoveNode(name)
	s.mu.Unlock()

	writeJSON(w, map[string]string{"status": "removed", "name": name})
}

// GET /route?key=...
func (s *server) route(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		http.Error(w, "key query parameter is required", http.StatusBadRequest)
		return
	}

	node := s.ring.GetNode(key)
	if node == nil {
		http.Error(w, "no nodes in ring", http.StatusServiceUnavailable)
		return
	}

	// Track this key in our sample set for distribution stats
	s.mu.Lock()
	if len(s.tracked) < 10_000 {
		s.tracked = append(s.tracked, key)
	}
	s.mu.Unlock()

	writeJSON(w, routeResponse{Node: node.Name, Addr: node.Addr})
}

// GET /stats
func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.Lock()
	keys := make([]string, len(s.tracked))
	copy(keys, s.tracked)
	nodeCount := s.ring.NodeCount()
	s.mu.Unlock()

	if len(keys) == 0 || nodeCount == 0 {
		writeJSON(w, statsResponse{Nodes: nodeCount, KeysPerNode: map[string]int{}})
		return
	}

	dist := s.ring.Distribution(keys)
	writeJSON(w, statsResponse{
		Nodes:       nodeCount,
		KeysPerNode: dist.KeysPerNode,
		StdDev:      math.Round(dist.StdDev*1000) / 1000,
		Min:         dist.Min,
		Max:         dist.Max,
	})
}

// GET /health
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, healthResponse{Status: "ok", Nodes: s.ring.NodeCount()})
}

// ── Routing ───────────────────────────────────────────────────────────────────

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/nodes", s.addNode)
	mux.HandleFunc("/nodes/", s.removeNode)
	mux.HandleFunc("/route", s.route)
	mux.HandleFunc("/stats", s.stats)
	mux.HandleFunc("/health", s.health)

	return mux
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	port   := flag.Int("port", 8080, "HTTP port to listen on")
	vnodes := flag.Int("vnodes", 100, "Virtual nodes per physical node")
	flag.Parse()

	srv := &server{
		ring:    ring.NewManaged(*vnodes),
		tracked: make([]string, 0, 10_000),
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("hash ring server starting on %s (vnodes=%d)", addr, *vnodes)
	log.Printf("endpoints: POST /nodes  DELETE /nodes/{name}  GET /route?key=  GET /stats  GET /health")

	if err := http.ListenAndServe(addr, srv.routes()); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}
