package pubsub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ── Message ───────────────────────────────────────────────────────────────────

// Message is the unit of data flowing through the broker.
// Attributes allow subscribers to filter without parsing Body.
// OrderingKey (v2) groups related messages for in-order delivery.
type Message struct {
	ID          string
	Topic       string
	Body        []byte
	Attributes  map[string]string
	OrderingKey string
	PublishedAt time.Time
}

// newID generates a random 16-byte hex ID (UUID-like, no external dependency).
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fallback to time-based ID if crypto/rand fails (should not happen).
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// newMessage creates a Message with a fresh random ID and timestamp.
func newMessage(topic string, body []byte, attrs map[string]string, orderingKey string) Message {
	return Message{
		ID:          newID(),
		Topic:       topic,
		Body:        body,
		Attributes:  attrs,
		OrderingKey: orderingKey,
		PublishedAt: time.Now(),
	}
}

// ── MessageFilter ─────────────────────────────────────────────────────────────

// MessageFilter is a predicate a subscriber uses to receive only matching messages.
// Return true to deliver the message, false to skip it.
// A nil filter always delivers.
type MessageFilter func(Message) bool

// ── Subscription ──────────────────────────────────────────────────────────────

// SubscriptionType determines how the broker delivers messages.
type SubscriptionType int

const (
	Pull SubscriptionType = iota // subscriber reads from channel
	Push                         // broker POSTs to webhookURL
)

// BackpressurePolicy controls what the broker does when a subscription's
// buffer is full.
type BackpressurePolicy int

const (
	Drop  BackpressurePolicy = iota // silently drop the message
	Block                           // publisher blocks until buffer has space
)

// Subscription represents one subscriber's connection to a topic.
// It is created by Broker.Subscribe and destroyed by Broker.Unsubscribe.
type Subscription struct {
	ID         string
	Topic      string
	Filter     MessageFilter
	Type       SubscriptionType
	WebhookURL string // non-empty when Type == Push

	// Pull-mode delivery channel (v0/v1).
	ch chan Message

	// Backpressure policy (v1).
	backpressure BackpressurePolicy

	// Stats (v1) — atomic via sync/atomic in delivery.go
	stats SubscriptionStats

	// Async delivery (v1): goroutine lifecycle.
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Ordering (v2): per-key serialised dispatch.
	orderingMu   sync.Mutex
	orderingKeys map[string]chan Message // orderingKey → serialised channel

	// Delivery callback (test seam, v1): nil = always succeed.
	// If non-nil and returns false, delivery is retried per the backoff schedule.
	callback func(Message) bool

	// broker reference for DLQ publishing (v1).
	broker *Broker
}

// ── Topic ─────────────────────────────────────────────────────────────────────

// Topic holds the set of active subscriptions and the per-topic ACL.
type Topic struct {
	name          string
	mu            sync.RWMutex
	subscribers   map[string]*Subscription
	acl           *ACL // nil means "allow all" (v2)
}

// ── Broker ────────────────────────────────────────────────────────────────────

// Broker is the central pub/sub coordinator.
// All public methods are safe for concurrent use.
type Broker struct {
	mu     sync.RWMutex
	topics map[string]*Topic

	// v1: async mode flag — set by NewAsyncBroker.
	async bool
}

// NewBroker creates a synchronous broker (v0).
// Publish blocks until every subscriber has received the message.
func NewBroker() *Broker {
	return &Broker{
		topics: make(map[string]*Topic),
		async:  false,
	}
}

// NewAsyncBroker creates an async broker (v1+).
// Publish returns as soon as each subscription's buffer has accepted the message.
func NewAsyncBroker() *Broker {
	return &Broker{
		topics: make(map[string]*Topic),
		async:  true,
	}
}

// getOrCreateTopic returns an existing topic or creates a new one.
func (b *Broker) getOrCreateTopic(name string) *Topic {
	b.mu.RLock()
	t, ok := b.topics[name]
	b.mu.RUnlock()
	if ok {
		return t
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if t, ok = b.topics[name]; ok {
		return t
	}
	t = &Topic{
		name:        name,
		subscribers: make(map[string]*Subscription),
	}
	b.topics[name] = t
	return t
}

// CreateTopic explicitly creates a topic (with an optional ACL for v2).
// Topics are also created implicitly on first Subscribe/Publish.
func (b *Broker) CreateTopic(name string, acl *ACL) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.topics[name]; !ok {
		b.topics[name] = &Topic{
			name:        name,
			subscribers: make(map[string]*Subscription),
			acl:         acl,
		}
	}
}

// Subscribe creates a new subscription on the named topic.
// filter may be nil (all messages delivered).
// Returns the new Subscription (use sub.ch to receive messages in Pull mode).
func (b *Broker) Subscribe(topic string, filter MessageFilter) *Subscription {
	t := b.getOrCreateTopic(topic)

	sub := &Subscription{
		ID:           newID(),
		Topic:        topic,
		Filter:       filter,
		Type:         Pull,
		ch:           make(chan Message, 1000),
		backpressure: Drop,
		stopCh:       make(chan struct{}),
		orderingKeys: make(map[string]chan Message),
		broker:       b,
	}

	if b.async {
		sub.wg.Add(1)
		go sub.deliveryLoop()
	}

	t.mu.Lock()
	t.subscribers[sub.ID] = sub
	t.mu.Unlock()

	return sub
}

// SubscribeWithOptions creates a subscription with explicit backpressure and type settings.
func (b *Broker) SubscribeWithOptions(topic string, filter MessageFilter, policy BackpressurePolicy, subType SubscriptionType, webhookURL string) *Subscription {
	sub := b.Subscribe(topic, filter)
	sub.backpressure = policy
	sub.Type = subType
	sub.WebhookURL = webhookURL

	if subType == Push && b.async {
		sub.wg.Add(1)
		go sub.pushLoop()
	}

	return sub
}

// Unsubscribe removes the subscription with the given ID.
// In async mode the delivery goroutine is stopped before returning.
func (b *Broker) Unsubscribe(subscriptionID string) {
	b.mu.RLock()
	var found *Subscription
	var foundTopic *Topic
	for _, t := range b.topics {
		t.mu.RLock()
		sub, ok := t.subscribers[subscriptionID]
		t.mu.RUnlock()
		if ok {
			found = sub
			foundTopic = t
			break
		}
	}
	b.mu.RUnlock()

	if found == nil {
		return
	}

	foundTopic.mu.Lock()
	delete(foundTopic.subscribers, subscriptionID)
	foundTopic.mu.Unlock()

	// Stop async goroutine.
	close(found.stopCh)
	found.wg.Wait()
}

// Publish sends msg to all current subscribers of the named topic.
// In synchronous mode (v0) it blocks until all subscribers have received the message.
// In async mode (v1+) it returns as soon as each subscriber's buffer is updated.
// Returns the message ID.
func (b *Broker) Publish(topicName string, body []byte, attrs map[string]string) (string, error) {
	return b.PublishOrdered(topicName, body, attrs, "")
}

// PublishOrdered sends a message with an ordering key (v2).
// Messages with the same ordering key are delivered to each subscriber in publish order.
func (b *Broker) PublishOrdered(topicName string, body []byte, attrs map[string]string, orderingKey string) (string, error) {
	t := b.getOrCreateTopic(topicName)

	msg := newMessage(topicName, body, attrs, orderingKey)

	t.mu.RLock()
	subs := make([]*Subscription, 0, len(t.subscribers))
	for _, s := range t.subscribers {
		subs = append(subs, s)
	}
	t.mu.RUnlock()

	for _, sub := range subs {
		if sub.Filter != nil && !sub.Filter(msg) {
			continue
		}

		if b.async {
			deliverAsync(sub, msg)
		} else {
			// Synchronous: publisher blocks until this subscriber's channel send completes.
			sub.ch <- msg
		}
	}

	return msg.ID, nil
}

// Ch returns the message channel for a Pull subscription.
// Callers read from this channel to consume delivered messages.
func (s *Subscription) Ch() <-chan Message {
	return s.ch
}

// Stats returns a snapshot of delivery statistics for this subscription (v1+).
func (s *Subscription) Stats() SubscriptionStats {
	return s.stats.snapshot()
}

// Close releases subscription resources. Equivalent to calling Broker.Unsubscribe.
func (s *Subscription) Close() {
	select {
	case <-s.stopCh:
		// Already closed.
	default:
		close(s.stopCh)
		s.wg.Wait()
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// Topics returns the names of all topics currently registered with the broker.
func (b *Broker) Topics() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	names := make([]string, 0, len(b.topics))
	for name := range b.topics {
		names = append(names, name)
	}
	return names
}

// SubscriberCount returns the number of active subscribers on a topic.
func (b *Broker) SubscriberCount(topicName string) int {
	b.mu.RLock()
	t, ok := b.topics[topicName]
	b.mu.RUnlock()
	if !ok {
		return 0
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.subscribers)
}

// validateACL checks whether a clientID is allowed to publish/subscribe.
// Returns nil if allowed or if no ACL is set.
func (b *Broker) validatePublishACL(topicName, clientID string) error {
	b.mu.RLock()
	t, ok := b.topics[topicName]
	b.mu.RUnlock()
	if !ok || t.acl == nil {
		return nil
	}
	if !t.acl.AllowPublish(clientID) {
		return fmt.Errorf("clientID %q is not permitted to publish to topic %q", clientID, topicName)
	}
	return nil
}

func (b *Broker) validateSubscribeACL(topicName, clientID string) error {
	b.mu.RLock()
	t, ok := b.topics[topicName]
	b.mu.RUnlock()
	if !ok || t.acl == nil {
		return nil
	}
	if !t.acl.AllowSubscribe(clientID) {
		return fmt.Errorf("clientID %q is not permitted to subscribe to topic %q", clientID, topicName)
	}
	return nil
}

// PublishAs publishes a message with ACL enforcement (v2).
func (b *Broker) PublishAs(clientID, topicName string, body []byte, attrs map[string]string) (string, error) {
	if err := b.validatePublishACL(topicName, clientID); err != nil {
		return "", err
	}
	return b.Publish(topicName, body, attrs)
}

// SubscribeAs creates a subscription with ACL enforcement (v2).
func (b *Broker) SubscribeAs(clientID, topic string, filter MessageFilter) (*Subscription, error) {
	if err := b.validateSubscribeACL(topic, clientID); err != nil {
		return nil, err
	}
	return b.Subscribe(topic, filter), nil
}
