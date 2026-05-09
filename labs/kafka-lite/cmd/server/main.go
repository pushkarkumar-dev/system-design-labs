// Kafka-lite HTTP server
//
// Exposes the disk broker over a simple REST API. Any language can produce and
// consume messages, and manage consumer group offsets, without a native Kafka
// client library.
//
// Routes:
//
//	POST   /produce                         — produce a message
//	GET    /consume                         — consume messages from a topic
//	POST   /groups/{groupId}/join           — join or create a consumer group
//	POST   /groups/{groupId}/heartbeat      — keep-alive (must call every 3s)
//	POST   /groups/{groupId}/commit         — commit an offset for a partition
//	GET    /groups/{groupId}/offset         — fetch committed offset
//	GET    /health                          — liveness check
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"dev.pushkar/kafka-lite/pkg/broker"
)

// ── Server ─────────────────────────────────────────────────────────────────────

type server struct {
	b *broker.DiskBroker
}

// ── Request / Response types ───────────────────────────────────────────────────

type produceRequest struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Message   string `json:"message"` // base64 or plain text
}

type produceResponse struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type consumeResponse struct {
	Messages []messageDTO `json:"messages"`
}

type messageDTO struct {
	Offset  int64  `json:"offset"`
	Payload string `json:"payload"`
}

type joinRequest struct {
	ClientID string `json:"clientId"`
}

type joinResponse struct {
	GroupID  string `json:"groupId"`
	MemberID string `json:"memberId"`
}

type commitRequest struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type offsetRequest struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
}

type offsetResponse struct {
	Topic     string `json:"topic"`
	Partition int    `json:"partition"`
	Offset    int64  `json:"offset"`
}

type healthResponse struct {
	Status string `json:"status"`
}

// ── Handlers ───────────────────────────────────────────────────────────────────

// POST /produce
func (s *server) handleProduce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req produceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Topic == "" {
		http.Error(w, "topic is required", http.StatusBadRequest)
		return
	}

	offset, err := s.b.Produce(req.Topic, req.Partition, []byte(req.Message))
	if err != nil {
		http.Error(w, "produce failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, produceResponse{
		Topic:     req.Topic,
		Partition: req.Partition,
		Offset:    offset,
	})
}

// GET /consume?topic=&partition=&offset=&limit=
func (s *server) handleConsume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	topic := r.URL.Query().Get("topic")
	if topic == "" {
		http.Error(w, "topic query parameter is required", http.StatusBadRequest)
		return
	}

	partition := 0
	if p := r.URL.Query().Get("partition"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil {
			http.Error(w, "invalid partition", http.StatusBadRequest)
			return
		}
		partition = v
	}

	offset := int64(0)
	if o := r.URL.Query().Get("offset"); o != "" {
		v, err := strconv.ParseInt(o, 10, 64)
		if err != nil {
			http.Error(w, "invalid offset", http.StatusBadRequest)
			return
		}
		offset = v
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil || v <= 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		limit = v
	}

	msgs, err := s.b.Consume(topic, partition, offset, limit)
	if err != nil {
		http.Error(w, "consume failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	dtos := make([]messageDTO, len(msgs))
	for i, m := range msgs {
		dtos[i] = messageDTO{
			Offset:  m.Offset,
			Payload: string(m.Payload),
		}
	}

	writeJSON(w, consumeResponse{Messages: dtos})
}

// POST /groups/{groupId}/join
func (s *server) handleJoinGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.ClientID == "" {
		req.ClientID = "anonymous"
	}

	memberID, err := s.b.JoinGroup(groupID, req.ClientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, joinResponse{GroupID: groupID, MemberID: memberID})
}

// POST /groups/{groupId}/heartbeat
func (s *server) handleHeartbeat(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	memberID := r.URL.Query().Get("memberId")
	if memberID == "" {
		http.Error(w, "memberId query parameter is required", http.StatusBadRequest)
		return
	}

	if err := s.b.Heartbeat(groupID, memberID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

// POST /groups/{groupId}/commit
func (s *server) handleCommit(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req commitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.b.CommitOffset(groupID, req.Topic, req.Partition, req.Offset); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"group":     groupID,
		"topic":     req.Topic,
		"partition": req.Partition,
		"offset":    req.Offset,
		"status":    "committed",
	})
}

// GET /groups/{groupId}/offset?topic=&partition=
func (s *server) handleFetchOffset(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	topic := r.URL.Query().Get("topic")
	if topic == "" {
		http.Error(w, "topic query parameter is required", http.StatusBadRequest)
		return
	}

	partition := 0
	if p := r.URL.Query().Get("partition"); p != "" {
		v, err := strconv.Atoi(p)
		if err != nil {
			http.Error(w, "invalid partition", http.StatusBadRequest)
			return
		}
		partition = v
	}

	off, err := s.b.FetchOffset(groupID, topic, partition)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, offsetResponse{
		Topic:     topic,
		Partition: partition,
		Offset:    off,
	})
}

// GET /health
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, healthResponse{Status: "ok"})
}

// ── Routing ─────────────────────────────────────────────────────────────────────

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/produce", s.handleProduce)
	mux.HandleFunc("/consume", s.handleConsume)
	mux.HandleFunc("/health", s.handleHealth)

	// Route /groups/{groupId}/* to the appropriate handler.
	mux.HandleFunc("/groups/", func(w http.ResponseWriter, r *http.Request) {
		// Path: /groups/{groupId}/{action}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/groups/"), "/")
		if len(parts) < 2 {
			http.Error(w, "invalid groups path", http.StatusBadRequest)
			return
		}
		groupID := parts[0]
		action := parts[1]

		switch action {
		case "join":
			s.handleJoinGroup(w, r, groupID)
		case "heartbeat":
			s.handleHeartbeat(w, r, groupID)
		case "commit":
			s.handleCommit(w, r, groupID)
		case "offset":
			s.handleFetchOffset(w, r, groupID)
		default:
			http.Error(w, fmt.Sprintf("unknown action %q", action), http.StatusNotFound)
		}
	})

	return mux
}

// ── Main ─────────────────────────────────────────────────────────────────────────

func main() {
	port    := flag.Int("port", 8080, "HTTP port to listen on")
	dataDir := flag.String("data", "/tmp/kafka-lite-data", "Directory to store topic segments")
	flag.Parse()

	b, err := broker.NewDiskBroker(*dataDir)
	if err != nil {
		log.Fatalf("failed to open broker at %s: %v", *dataDir, err)
	}

	srv := &server{b: b}
	addr := fmt.Sprintf(":%d", *port)

	log.Printf("kafka-lite server starting on %s (data=%s)", addr, *dataDir)
	log.Printf("endpoints: POST /produce  GET /consume  /groups/{id}/join|heartbeat|commit|offset  GET /health")

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
