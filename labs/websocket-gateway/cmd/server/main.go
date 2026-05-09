// WebSocket pub/sub gateway server.
//
// Exposes the hub over HTTP/WebSocket. Clients connect with a standard
// WebSocket handshake and exchange JSON-framed messages:
//
// Client → Server:
//
//	{"action":"join","room":"lobby"}
//	{"action":"leave"}
//	{"action":"message","content":"hello everyone"}
//	{"action":"ack","seqno":42}
//
// Server → Client:
//
//	{"event":"presence","room":"lobby","user":"alice","action":"joined","members":["alice","bob"]}
//	{"event":"message","room":"lobby","user":"bob","content":"hello","seqno":1}
//
// Routes:
//
//	GET /ws          — WebSocket upgrade endpoint
//	GET /stats       — hub statistics (JSON)
//	GET /health      — liveness check
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"

	ws "github.com/pushkar1005/system-design-labs/labs/websocket-gateway/pkg/ws"
)

func main() {
	port     := flag.Int("port", 8080, "HTTP port to listen on")
	maxConns := flag.Int("max-conns", 10000, "Maximum concurrent WebSocket connections")
	flag.Parse()

	hub := ws.NewHub(*maxConns)

	mux := http.NewServeMux()

	// WebSocket upgrade endpoint.
	mux.Handle("/ws", hub)

	// Stats endpoint — for monitoring and smoke tests.
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := hub.Stats()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"totalConnected":  stats.TotalConnected,
			"totalMessages":   stats.TotalMessages,
			"droppedMessages": stats.DroppedMessages,
			"activeRooms":     stats.ActiveRooms,
			"currentConnections": hub.ConnectedCount(),
			"currentRooms":       hub.ActiveRoomCount(),
		})
	})

	// Health check.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("websocket-gateway starting on %s (max-conns=%d)", addr, *maxConns)
	log.Printf("endpoints: GET /ws  GET /stats  GET /health")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
