package pubsub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// ── Push subscriptions (v2) ───────────────────────────────────────────────────
//
// A push subscription has the broker deliver messages to the subscriber's HTTP
// endpoint via POST. The broker expects an HTTP 200 response to acknowledge
// delivery. Any non-200 response triggers a retry with exponential backoff
// (same schedule as v1 async retry: 1s, 2s, 4s, max 3 retries).
//
// This mirrors Google Cloud Pub/Sub push subscriptions and AWS SNS HTTP
// endpoints. The trade-off vs. pull:
//   - Push: lower latency (broker drives delivery), but subscriber must be
//     reachable over HTTP, and slow subscribers create back-pressure on the
//     broker's goroutines.
//   - Pull: subscriber controls rate, works behind NAT, better for batching.
//
// Wire format: POST to webhookURL with body:
//   {"id":"<msgID>","topic":"<topic>","body":"<base64>","attributes":{...},"publishedAt":"<RFC3339>"}

// pushPayload is the JSON body sent to push subscribers.
type pushPayload struct {
	ID          string            `json:"id"`
	Topic       string            `json:"topic"`
	Body        []byte            `json:"body"` // base64-encoded by json.Marshal
	Attributes  map[string]string `json:"attributes"`
	PublishedAt time.Time         `json:"publishedAt"`
}

// pushLoop is the goroutine that drives push delivery for a Push subscription.
// It reads from sub.ch (messages are placed there by deliverAsync) and POSTs
// each message to sub.WebhookURL.
func (sub *Subscription) pushLoop() {
	defer sub.wg.Done()

	client := &http.Client{Timeout: 10 * time.Second}

	for {
		select {
		case <-sub.stopCh:
			return
		case msg, ok := <-sub.ch:
			if !ok {
				return
			}
			sub.pushWithRetry(client, msg)
		}
	}
}

// pushWithRetry attempts to POST msg to the subscriber's webhook URL.
// On non-200 response, retries with exponential backoff (max 3 retries).
// After all retries exhausted, publishes to the DLQ topic.
func (sub *Subscription) pushWithRetry(client *http.Client, msg Message) {
	payload := pushPayload{
		ID:          msg.ID,
		Topic:       msg.Topic,
		Body:        msg.Body,
		Attributes:  msg.Attributes,
		PublishedAt: msg.PublishedAt,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		atomic.AddInt64(&sub.stats.DLQ, 1)
		return
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			atomic.AddInt64(&sub.stats.Retried, 1)
			delay := retryDelays[attempt-1]
			select {
			case <-sub.stopCh:
				return
			case <-time.After(delay):
			}
		}

		if postToWebhook(client, sub.WebhookURL, data) {
			atomic.AddInt64(&sub.stats.Delivered, 1)
			return
		}
	}

	// All retries exhausted.
	atomic.AddInt64(&sub.stats.DLQ, 1)
	if sub.broker != nil {
		dlqTopic := sub.Topic + "-dlq"
		_, _ = sub.broker.Publish(dlqTopic, msg.Body, msg.Attributes)
	}
}

// postToWebhook sends a POST request to url with the given JSON body.
// Returns true on HTTP 200, false on any error or non-200 status.
func postToWebhook(client *http.Client, url string, body []byte) bool {
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// RegisterPushSubscription is a convenience wrapper that creates a push
// subscription and returns it. The push goroutine is started automatically.
func (b *Broker) RegisterPushSubscription(topic, webhookURL string, filter MessageFilter) (*Subscription, error) {
	if webhookURL == "" {
		return nil, fmt.Errorf("webhookURL must be non-empty for push subscriptions")
	}
	sub := b.SubscribeWithOptions(topic, filter, Drop, Push, webhookURL)
	return sub, nil
}
