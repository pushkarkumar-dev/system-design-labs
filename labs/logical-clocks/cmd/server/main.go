// Logical Clocks HTTP Server
//
// Exposes all three logical clock implementations over a REST API.
// Any language or tool can interact with the clocks via HTTP — useful
// for multi-process experiments where you want to observe causality.
//
// Routes:
//
//	POST   /lamport/tick              — increment; returns {"ts": N}
//	POST   /lamport/send              — tick + return timestamp to attach to message
//	POST   /lamport/receive           — receive message: body {"ts": N}
//	GET    /lamport/value             — current counter: {"ts": N}
//
//	POST   /vector/tick               — increment own slot
//	POST   /vector/send               — tick + return full vector
//	POST   /vector/receive            — receive vector: body {"vector": {...}}
//	GET    /vector/value              — current vector
//	POST   /vector/happens-before     — body {"a": {...}, "b": {...}}; returns bool
//	POST   /vector/concurrent         — body {"a": {...}, "b": {...}}; returns bool
//
//	GET    /hlc/now                   — generate new HLC timestamp
//	POST   /hlc/receive               — receive HLC: body {"wall": N, "counter": N}
//	GET    /hlc/value                 — current HLC without advancing
//
//	GET    /health                    — liveness check
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	"dev.pushkar/logical-clocks/pkg/clocks"
)

// ── Server state ──────────────────────────────────────────────────────────────

type server struct {
	lamport *clocks.LamportClock
	vector  *clocks.VectorClock
	hlc     *clocks.HLC
	nodeID  string
}

// ── Request / Response types ──────────────────────────────────────────────────

type lamportValueResponse struct {
	Ts uint64 `json:"ts"`
}

type lamportReceiveRequest struct {
	Ts uint64 `json:"ts"`
}

type vectorValueResponse struct {
	Vector map[string]uint64 `json:"vector"`
}

type vectorReceiveRequest struct {
	Vector map[string]uint64 `json:"vector"`
}

type happensBefore2Request struct {
	A map[string]uint64 `json:"a"`
	B map[string]uint64 `json:"b"`
}

type boolResponse struct {
	Result bool `json:"result"`
}

type hlcRequest struct {
	Wall    int64  `json:"wall"`
	Counter uint16 `json:"counter"`
}

type hlcResponse struct {
	Wall    int64  `json:"wall"`
	Counter uint16 `json:"counter"`
}

type healthResponse struct {
	Status string `json:"status"`
	NodeID string `json:"nodeId"`
}

// ── Lamport handlers ──────────────────────────────────────────────────────────

func (s *server) lamportTick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	ts := s.lamport.Tick()
	writeJSON(w, lamportValueResponse{Ts: ts})
}

func (s *server) lamportSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	ts := s.lamport.Send()
	writeJSON(w, lamportValueResponse{Ts: ts})
}

func (s *server) lamportReceive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req lamportReceiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.lamport.Receive(req.Ts)
	writeJSON(w, lamportValueResponse{Ts: s.lamport.Value()})
}

func (s *server) lamportValue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, lamportValueResponse{Ts: s.lamport.Value()})
}

// ── Vector clock handlers ─────────────────────────────────────────────────────

func (s *server) vectorTick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	s.vector.Tick()
	writeJSON(w, vectorValueResponse{Vector: s.vector.Vector()})
}

func (s *server) vectorSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	vec := s.vector.Send()
	writeJSON(w, vectorValueResponse{Vector: vec})
}

func (s *server) vectorReceive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req vectorReceiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.vector.Receive(req.Vector)
	writeJSON(w, vectorValueResponse{Vector: s.vector.Vector()})
}

func (s *server) vectorValue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, vectorValueResponse{Vector: s.vector.Vector()})
}

func (s *server) vectorHappensBefore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req happensBefore2Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, boolResponse{Result: clocks.HappensBefore(req.A, req.B)})
}

func (s *server) vectorConcurrent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req happensBefore2Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, boolResponse{Result: clocks.Concurrent(req.A, req.B)})
}

// ── HLC handlers ──────────────────────────────────────────────────────────────

func (s *server) hlcNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	ts := s.hlc.Now()
	writeJSON(w, hlcResponse{Wall: ts.Wall, Counter: ts.Counter})
}

func (s *server) hlcReceive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req hlcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	ts := s.hlc.Receive(clocks.HLCTimestamp{Wall: req.Wall, Counter: req.Counter})
	writeJSON(w, hlcResponse{Wall: ts.Wall, Counter: ts.Counter})
}

func (s *server) hlcValue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	ts := s.hlc.Timestamp()
	writeJSON(w, hlcResponse{Wall: ts.Wall, Counter: ts.Counter})
}

// ── Health ────────────────────────────────────────────────────────────────────

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, healthResponse{Status: "ok", NodeID: s.nodeID})
}

// ── Routing ───────────────────────────────────────────────────────────────────

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	// Lamport
	mux.HandleFunc("/lamport/tick", s.lamportTick)
	mux.HandleFunc("/lamport/send", s.lamportSend)
	mux.HandleFunc("/lamport/receive", s.lamportReceive)
	mux.HandleFunc("/lamport/value", s.lamportValue)

	// Vector
	mux.HandleFunc("/vector/tick", s.vectorTick)
	mux.HandleFunc("/vector/send", s.vectorSend)
	mux.HandleFunc("/vector/receive", s.vectorReceive)
	mux.HandleFunc("/vector/value", s.vectorValue)
	mux.HandleFunc("/vector/happens-before", s.vectorHappensBefore)
	mux.HandleFunc("/vector/concurrent", s.vectorConcurrent)

	// HLC
	mux.HandleFunc("/hlc/now", s.hlcNow)
	mux.HandleFunc("/hlc/receive", s.hlcReceive)
	mux.HandleFunc("/hlc/value", s.hlcValue)

	// Health
	mux.HandleFunc("/health", s.health)

	return mux
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	port   := flag.Int("port", 8080, "HTTP port to listen on")
	nodeID := flag.String("node-id", "node-1", "Node ID used as the vector clock key")
	flag.Parse()

	srv := &server{
		lamport: clocks.NewLamport(),
		vector:  clocks.NewVector(*nodeID),
		hlc:     clocks.NewHLC(),
		nodeID:  *nodeID,
	}

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("logical clocks server starting on %s (node-id=%s)", addr, *nodeID)
	log.Printf("lamport:  POST /lamport/tick  POST /lamport/send  POST /lamport/receive  GET /lamport/value")
	log.Printf("vector:   POST /vector/tick   POST /vector/send   POST /vector/receive   GET /vector/value")
	log.Printf("hlc:      GET  /hlc/now       POST /hlc/receive   GET /hlc/value")
	log.Printf("health:   GET  /health")

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
