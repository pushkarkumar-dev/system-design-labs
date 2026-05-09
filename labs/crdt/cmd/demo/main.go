// Demo program for the CRDT library.
//
// Simulates two distributed nodes independently incrementing a GCounter,
// then merging to demonstrate convergence. Also demos ORSet re-add semantics
// and the delta-state bandwidth reduction.
//
// Usage:
//
//	go run ./cmd/demo
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/pushkar1005/system-design-labs/labs/crdt/pkg/crdt"
)

func main() {
	fmt.Println("=== CRDT Library Demo ===")
	fmt.Println()

	demoGCounter()
	demoPNCounter()
	demoORSet()
	demoDeltaCompression()

	// Start HTTP server for Java integration.
	srv := startHTTPServer()
	fmt.Println()
	fmt.Println("HTTP demo server running on :8090")
	fmt.Println("  GET  /counter       — read global GCounter value")
	fmt.Println("  POST /counter/inc   — increment nodeA or nodeB (body: node=nodeA)")
	fmt.Println("  GET  /orset         — list ORSet elements")
	fmt.Println("  POST /orset/add     — add element (body: elem=hello&node=nodeA)")
	fmt.Println("  POST /orset/remove  — remove element (body: elem=hello)")
	fmt.Println("  GET  /health        — liveness check")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop.")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	_ = srv.Close()
}

// ─────────────────────────────────────────────────────────────────────────────
// Demo functions
// ─────────────────────────────────────────────────────────────────────────────

func demoGCounter() {
	fmt.Println("── GCounter: grow-only counter ──────────────────────")

	nodeA := crdt.NewGCounter()
	nodeB := crdt.NewGCounter()

	nodeA.Increment("nodeA")
	nodeA.Increment("nodeA")
	nodeA.Increment("nodeA") // nodeA = 3
	nodeB.Increment("nodeB")
	nodeB.Increment("nodeB") // nodeB = 2

	fmt.Printf("  nodeA sees: %d  nodeB sees: %d  (before merge)\n",
		nodeA.Value(), nodeB.Value())

	// Merge: each sees both increments.
	nodeA.Merge(nodeB)
	nodeB.Merge(nodeA)

	fmt.Printf("  nodeA sees: %d  nodeB sees: %d  (after merge — converged!)\n",
		nodeA.Value(), nodeB.Value())
	fmt.Println()
}

func demoPNCounter() {
	fmt.Println("── PNCounter: positive-negative counter ─────────────")

	p := crdt.NewPNCounter()
	p.Increment("n1")
	p.Increment("n1")
	p.Decrement("n1")
	p.Decrement("n2")
	p.Decrement("n2")

	fmt.Printf("  Value after +1+1-1-1-1 = %d (expected -1)\n", p.Value())
	fmt.Println()
}

func demoORSet() {
	fmt.Println("── ORSet: observed-remove set ───────────────────────")

	s := crdt.NewORSet[string]()
	s.Add("nodeA", "alice")
	s.Add("nodeA", "bob")
	fmt.Printf("  After adding alice,bob: %v\n", s.Elements())

	s.Remove("alice")
	fmt.Printf("  After removing alice: contains(alice)=%v\n", s.Contains("alice"))

	// Key property: re-add after remove works (unlike 2P-Set).
	s.Add("nodeB", "alice")
	fmt.Printf("  After re-adding alice: contains(alice)=%v (ORSet allows re-add!)\n",
		s.Contains("alice"))

	// Concurrent add + remove across two replicas.
	a := crdt.NewORSet[string]()
	b := crdt.NewORSet[string]()
	a.Add("nodeA", "x")
	b.Merge(a) // b sees the initial add

	a.Remove("x")         // A removes x
	b.Add("nodeB", "x")   // B concurrently re-adds x with new tag

	a.Merge(b)
	fmt.Printf("  Concurrent add+remove — after merge: contains(x)=%v (add-wins)\n",
		a.Contains("x"))
	fmt.Println()
}

func demoDeltaCompression() {
	fmt.Println("── Delta-state compression ──────────────────────────")

	// Simulate a 100-node cluster.
	const nodeCount = 100
	receiver := crdt.NewDeltaGCounter("receiver")
	for i := 0; i < nodeCount-1; i++ {
		d := crdt.NewGCounter()
		d.IncrementBy(fmt.Sprintf("node%d", i), int64(i+1))
		receiver.ApplyDelta(d)
	}

	sender := crdt.NewDeltaGCounter("sender")
	delta := sender.Increment()
	fullState := receiver.FullState()

	deltaEntries, fullEntries := crdt.DeltaSize(delta, fullState)
	reduction := float64(fullEntries-deltaEntries) / float64(fullEntries) * 100

	fmt.Printf("  %d-node cluster, 1 increment:\n", nodeCount)
	fmt.Printf("    Full state: %d entries\n", fullEntries)
	fmt.Printf("    Delta:       %d entry\n", deltaEntries)
	fmt.Printf("    Reduction:  %.0f%%\n", reduction)
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP server for Java integration
// ─────────────────────────────────────────────────────────────────────────────

// shared state for the HTTP server.
var (
	sharedCounter crdt.GCounter
	sharedSet     crdt.ORSet[string]
)

func init() {
	sharedCounter = crdt.NewGCounter()
	sharedSet = crdt.NewORSet[string]()
}

func startHTTPServer() *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /counter", func(w http.ResponseWriter, r *http.Request) {
		val := sharedCounter.Value()
		entries := sharedCounter.Entries()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"value":%d,"entries":%s}`, val, entriesToJSON(entries))
	})

	mux.HandleFunc("POST /counter/inc", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		node := r.FormValue("node")
		if node == "" {
			node = "default"
		}
		sharedCounter.Increment(node)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"value":%d,"node":%q}`, sharedCounter.Value(), node)
	})

	mux.HandleFunc("GET /orset", func(w http.ResponseWriter, r *http.Request) {
		elems := sharedSet.Elements()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"elements":%s,"size":%d}`, elemsToJSON(elems), sharedSet.Size())
	})

	mux.HandleFunc("POST /orset/add", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		elem := r.FormValue("elem")
		node := r.FormValue("node")
		if elem == "" || node == "" {
			http.Error(w, "elem and node required", http.StatusBadRequest)
			return
		}
		sharedSet.Add(node, elem)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"added":%q,"size":%d}`, elem, sharedSet.Size())
	})

	mux.HandleFunc("POST /orset/remove", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		elem := r.FormValue("elem")
		if elem == "" {
			http.Error(w, "elem required", http.StatusBadRequest)
			return
		}
		sharedSet.Remove(elem)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"removed":%q,"size":%d}`, elem, sharedSet.Size())
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"up","service":"crdt-demo"}`)
	})

	srv := &http.Server{
		Addr:    ":8090",
		Handler: mux,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	return srv
}

func entriesToJSON(m map[string]int64) string {
	out := "{"
	first := true
	for k, v := range m {
		if !first {
			out += ","
		}
		out += fmt.Sprintf("%q:%d", k, v)
		first = false
	}
	return out + "}"
}

func elemsToJSON(elems []string) string {
	out := "["
	for i, e := range elems {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%q", e)
	}
	return out + "]"
}
