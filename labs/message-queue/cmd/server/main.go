// Command server exposes the message-queue package over a simple HTTP API.
//
// Endpoints:
//
//	POST   /queues                                 create a queue (body: {"name":"...", "dlqName":"...", "maxReceiveCount":3})
//	POST   /queues/{name}/messages                 send a message
//	GET    /queues/{name}/messages                 receive messages (?maxMessages=1&visibilityTimeout=30s&waitTime=0s)
//	DELETE /queues/{name}/messages/{receiptHandle} delete a message
//	GET    /queues/{name}/attributes               queue attributes
//
// This is deliberately minimal — no auth, no TLS, no rate limiting.
// It's a dev server for exploring the queue semantics interactively.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/message-queue/pkg/queue"
)

var manager = queue.NewManager()

func main() {
	port := flag.Int("port", 8080, "HTTP listen port")
	flag.Parse()

	// Register some default queues so the server is usable without a setup step.
	if _, err := manager.CreateQueue("default", nil); err != nil {
		log.Printf("creating default queue: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/queues", handleQueues)
	mux.HandleFunc("/queues/", handleQueueOps)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("message-queue server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// handleQueues handles POST /queues (create a queue).
func handleQueues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name            string `json:"name"`
		DLQName         string `json:"dlqName,omitempty"`
		MaxReceiveCount int    `json:"maxReceiveCount,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var cfg *queue.DLQConfig
	if req.DLQName != "" {
		cfg = &queue.DLQConfig{
			DLQName:         req.DLQName,
			MaxReceiveCount: req.MaxReceiveCount,
		}
		if cfg.MaxReceiveCount == 0 {
			cfg.MaxReceiveCount = 3
		}
	}

	q, err := manager.CreateQueue(req.Name, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"name": q.Name()})
}

// handleQueueOps routes sub-paths under /queues/{name}/...
func handleQueueOps(w http.ResponseWriter, r *http.Request) {
	// Path: /queues/{name}/messages[/{receiptHandle}]
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/queues/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}

	queueName := parts[0]
	resource := parts[1] // "messages" or "attributes"

	q := manager.Queue(queueName)
	if q == nil {
		http.Error(w, fmt.Sprintf("queue %q not found", queueName), http.StatusNotFound)
		return
	}

	switch resource {
	case "attributes":
		handleAttributes(w, r, q)
	case "messages":
		if len(parts) == 3 {
			// /queues/{name}/messages/{receiptHandle}
			handleDeleteMessage(w, r, q, parts[2])
		} else {
			switch r.Method {
			case http.MethodPost:
				handleSendMessage(w, r, q)
			case http.MethodGet:
				handleReceiveMessage(w, r, q)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		}
	default:
		http.NotFound(w, r)
	}
}

func handleSendMessage(w http.ResponseWriter, r *http.Request, q *queue.Queue) {
	var req struct {
		Body         string `json:"body"`
		DelaySeconds int    `json:"delaySeconds,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var id string
	if req.DelaySeconds > 0 {
		id = q.DelayedSendMessage([]byte(req.Body), time.Duration(req.DelaySeconds)*time.Second)
	} else {
		id = q.SendMessage([]byte(req.Body))
	}

	writeJSON(w, http.StatusCreated, map[string]string{"messageId": id})
}

func handleReceiveMessage(w http.ResponseWriter, r *http.Request, q *queue.Queue) {
	maxMessages := queryInt(r, "maxMessages", 1)
	visSec := queryFloat(r, "visibilityTimeout", 30.0)
	waitSec := queryFloat(r, "waitTime", 0.0)
	visTimeout := time.Duration(visSec * float64(time.Second))
	waitTime := time.Duration(waitSec * float64(time.Second))

	var msgs []queue.Message
	if waitTime > 0 {
		msgs = q.LongPollReceive(maxMessages, visTimeout, waitTime)
	} else {
		msgs = q.ReceiveMessage(maxMessages, visTimeout)
	}

	type msgResp struct {
		ID            string `json:"id"`
		Body          string `json:"body"`
		ReceiptHandle string `json:"receiptHandle"`
		ReceiveCount  int    `json:"receiveCount"`
	}
	resp := make([]msgResp, len(msgs))
	for i, m := range msgs {
		resp[i] = msgResp{
			ID:            m.ID,
			Body:          string(m.Body),
			ReceiptHandle: m.ReceiptHandle,
			ReceiveCount:  m.ReceiveCount,
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"messages": resp})
}

func handleDeleteMessage(w http.ResponseWriter, r *http.Request, q *queue.Queue, receiptHandle string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if q.DeleteMessage(receiptHandle) {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, "receipt handle not found or already expired", http.StatusNotFound)
	}
}

func handleAttributes(w http.ResponseWriter, r *http.Request, q *queue.Queue) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	attrs := q.Attributes()
	writeJSON(w, http.StatusOK, attrs)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func queryInt(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func queryFloat(r *http.Request, key string, def float64) float64 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}
