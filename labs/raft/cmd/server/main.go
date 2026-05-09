// Raft HTTP Server
//
// Starts N Raft nodes in the same process (useful for demos and the Java
// integration test).  Each node has its own HTTP port.
//
// Routes per node (port 8080 + nodeIndex):
//
//	GET  /state          — node state: {id, state, term, commitIndex, logLen}
//	GET  /log            — full in-memory log entries
//	POST /command        — submit a command to this node (redirects if not leader)
//	GET  /health         — liveness: {status:"ok", nodeId, isLeader}
//
// Usage:
//
//	go run ./cmd/server --nodes=3 --base-port=8080
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"dev.pushkar/raft/pkg/raft"
)

// ── Request / response types ──────────────────────────────────────────────────

type stateResponse struct {
	ID          int    `json:"id"`
	State       string `json:"state"`
	Term        int    `json:"term"`
	CommitIndex int    `json:"commitIndex"`
	LogLen      int    `json:"logLen"`
	IsLeader    bool   `json:"isLeader"`
}

type commandRequest struct {
	Cmd string `json:"cmd"`
}

type commandResponse struct {
	Accepted bool   `json:"accepted"`
	NodeID   int    `json:"nodeId"`
	Error    string `json:"error,omitempty"`
}

type healthResponse struct {
	Status   string `json:"status"`
	NodeID   int    `json:"nodeId"`
	IsLeader bool   `json:"isLeader"`
}

// ── per-node HTTP handler ─────────────────────────────────────────────────────

type nodeHandler struct {
	node *raft.Node
}

func (h *nodeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/state":
		h.handleState(w, r)
	case "/log":
		h.handleLog(w, r)
	case "/command":
		h.handleCommand(w, r)
	case "/health":
		h.handleHealth(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *nodeHandler) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	s := h.node.Status()
	writeJSON(w, stateResponse{
		ID:          s.ID,
		State:       s.State,
		Term:        s.Term,
		CommitIndex: s.CommitIndex,
		LogLen:      s.LogLen,
		IsLeader:    h.node.IsLeader(),
	})
}

func (h *nodeHandler) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.node.Log())
}

func (h *nodeHandler) handleCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Cmd == "" {
		http.Error(w, `"cmd" field required`, http.StatusBadRequest)
		return
	}
	if !h.node.IsLeader() {
		// Return 503 so the Java client knows to find the leader and retry.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(commandResponse{
			Accepted: false,
			NodeID:   h.node.Status().ID,
			Error:    "not the leader",
		})
		return
	}
	if !h.node.Submit(req.Cmd) {
		http.Error(w, "command queue full", http.StatusTooManyRequests)
		return
	}
	writeJSON(w, commandResponse{Accepted: true, NodeID: h.node.Status().ID})
}

func (h *nodeHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, healthResponse{
		Status:   "ok",
		NodeID:   h.node.Status().ID,
		IsLeader: h.node.IsLeader(),
	})
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	numNodes := flag.Int("nodes", 3, "Number of Raft nodes in the cluster")
	basePort := flag.Int("base-port", 8080, "First node listens on this port; subsequent nodes increment by 1")
	flag.Parse()

	log.Printf("starting %d-node Raft cluster (ports %d–%d)",
		*numNodes, *basePort, *basePort+*numNodes-1)

	cluster := raft.NewCluster(*numNodes)

	// Start one HTTP server per node.
	for i, node := range cluster.Nodes {
		port := *basePort + i
		handler := &nodeHandler{node: node}
		go func(p int, h http.Handler, id int) {
			addr := fmt.Sprintf(":%d", p)
			log.Printf("node %d HTTP server on %s", id, addr)
			if err := http.ListenAndServe(addr, h); err != nil {
				log.Fatalf("node %d server failed: %v", id, err)
			}
		}(port, handler, i)
	}

	// Wait for leader election and print a summary.
	leader, ok := cluster.WaitForLeader(2 * time.Second)
	if ok {
		log.Printf("leader elected: node %d (term %d)", leader.Status().ID, leader.Status().Term)
	} else {
		log.Printf("warning: no leader elected within 2 s")
	}

	// Block forever.
	select {}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}
