// Command server runs a simple HTTP pub/sub broker.
//
// Endpoints:
//
//	POST /topics                         — create a topic
//	POST /topics/{name}/publish          — publish a message
//	POST /subscriptions                  — create a pull subscription
//	GET  /subscriptions/{id}/pull        — pull one message
//	POST /subscriptions/{id}/push-config — register a push webhook
//	GET  /health                         — liveness probe
//
// Run:
//
//	go run ./cmd/server --port 8080
//
// Then from another terminal:
//
//	# Create topic
//	curl -s -X POST http://localhost:8080/topics -H 'Content-Type: application/json' -d '{"name":"orders"}'
//
//	# Subscribe
//	curl -s -X POST http://localhost:8080/subscriptions -H 'Content-Type: application/json' \
//	  -d '{"topic":"orders","id":"my-sub"}'
//
//	# Publish
//	curl -s -X POST http://localhost:8080/topics/orders/publish \
//	  -H 'Content-Type: application/json' \
//	  -d '{"body":"aGVsbG8=","attributes":{"type":"order"}}'
//
//	# Pull one message
//	curl -s http://localhost:8080/subscriptions/my-sub/pull
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/pubsub/pkg/pubsub"
)

var (
	port = flag.Int("port", 8080, "HTTP listen port")
)

type server struct {
	broker *pubsub.Broker

	mu   sync.RWMutex
	subs map[string]*pubsub.Subscription // subscriptionID → sub
}

func main() {
	flag.Parse()

	s := &server{
		broker: pubsub.NewAsyncBroker(),
		subs:   make(map[string]*pubsub.Subscription),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /topics", s.handleCreateTopic)
	mux.HandleFunc("POST /topics/{name}/publish", s.handlePublish)
	mux.HandleFunc("POST /subscriptions", s.handleCreateSubscription)
	mux.HandleFunc("GET /subscriptions/{id}/pull", s.handlePull)
	mux.HandleFunc("GET /subscriptions/{id}/stats", s.handleStats)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("pubsub broker listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createTopicRequest struct {
	Name string `json:"name"`
}

func (s *server) handleCreateTopic(w http.ResponseWriter, r *http.Request) {
	var req createTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "invalid request: 'name' required", http.StatusBadRequest)
		return
	}
	s.broker.CreateTopic(req.Name, nil)
	writeJSON(w, http.StatusCreated, map[string]string{"topic": req.Name, "status": "created"})
}

type publishRequest struct {
	Body        []byte            `json:"body"`        // raw bytes (base64 in JSON)
	Attributes  map[string]string `json:"attributes"`
	OrderingKey string            `json:"orderingKey"`
}

func (s *server) handlePublish(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "topic name required", http.StatusBadRequest)
		return
	}

	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	msgID, err := s.broker.PublishOrdered(name, req.Body, req.Attributes, req.OrderingKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"messageId": msgID,
		"topic":     name,
	})
}

type createSubscriptionRequest struct {
	Topic  string `json:"topic"`
	ID     string `json:"id"`     // optional custom ID
	Filter string `json:"filter"` // "attr:<key>=<value>" shorthand
}

func (s *server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Topic == "" {
		http.Error(w, "invalid request: 'topic' required", http.StatusBadRequest)
		return
	}

	var filter pubsub.MessageFilter
	if req.Filter != "" && strings.HasPrefix(req.Filter, "attr:") {
		// Simple "attr:key=value" filter shorthand.
		parts := strings.SplitN(strings.TrimPrefix(req.Filter, "attr:"), "=", 2)
		if len(parts) == 2 {
			key, val := parts[0], parts[1]
			filter = func(m pubsub.Message) bool {
				return m.Attributes[key] == val
			}
		}
	}

	sub := s.broker.Subscribe(req.Topic, filter)

	s.mu.Lock()
	s.subs[sub.ID] = sub
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]string{
		"subscriptionId": sub.ID,
		"topic":          req.Topic,
		"status":         "active",
	})
}

func (s *server) handlePull(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	sub, ok := s.subs[id]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}

	// Non-blocking pull — return immediately if no message is available.
	select {
	case msg := <-sub.Ch():
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":          msg.ID,
			"topic":       msg.Topic,
			"body":        msg.Body,
			"attributes":  msg.Attributes,
			"publishedAt": msg.PublishedAt.Format(time.RFC3339Nano),
		})
	default:
		writeJSON(w, http.StatusOK, map[string]interface{}{"message": nil})
	}
}

func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	sub, ok := s.subs[id]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "subscription not found", http.StatusNotFound)
		return
	}

	stats := sub.Stats()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscriptionId": id,
		"delivered":      stats.Delivered,
		"dropped":        stats.Dropped,
		"retried":        stats.Retried,
		"dlq":            stats.DLQ,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
