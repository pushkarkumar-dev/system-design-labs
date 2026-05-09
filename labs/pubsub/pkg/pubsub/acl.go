package pubsub

import "sync"

// ── ACL (v2) ─────────────────────────────────────────────────────────────────
//
// ACL enforces which clientIDs may publish to or subscribe from a topic.
// The model is a simple allow/deny list:
//
//   - If allowPublish is non-empty, only listed clientIDs may publish.
//     An empty allowPublish means "allow all publishers".
//   - If denyPublish contains a clientID, that client is always rejected,
//     even if listed in allowPublish.
//   - Same logic applies for subscribe.
//
// This mirrors AWS SNS resource-based policies at the topic level.

// ACL holds publisher and subscriber access control rules for one topic.
type ACL struct {
	mu sync.RWMutex

	allowPublish map[string]struct{}
	denyPublish  map[string]struct{}

	allowSubscribe map[string]struct{}
	denySubscribe  map[string]struct{}
}

// NewACL creates an empty ACL (allow all).
func NewACL() *ACL {
	return &ACL{
		allowPublish:   make(map[string]struct{}),
		denyPublish:    make(map[string]struct{}),
		allowSubscribe: make(map[string]struct{}),
		denySubscribe:  make(map[string]struct{}),
	}
}

// AllowPublisher adds clientID to the publish allow-list.
// Once any clientID is in the allow-list, only listed clients may publish.
func (a *ACL) AllowPublisher(clientID string) *ACL {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowPublish[clientID] = struct{}{}
	return a
}

// DenyPublisher adds clientID to the publish deny-list.
// Deny always wins over allow.
func (a *ACL) DenyPublisher(clientID string) *ACL {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.denyPublish[clientID] = struct{}{}
	return a
}

// AllowSubscriber adds clientID to the subscribe allow-list.
func (a *ACL) AllowSubscriber(clientID string) *ACL {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowSubscribe[clientID] = struct{}{}
	return a
}

// DenySubscriber adds clientID to the subscribe deny-list.
func (a *ACL) DenySubscriber(clientID string) *ACL {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.denySubscribe[clientID] = struct{}{}
	return a
}

// AllowPublish returns true if clientID is permitted to publish.
func (a *ACL) AllowPublish(clientID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.check(clientID, a.allowPublish, a.denyPublish)
}

// AllowSubscribe returns true if clientID is permitted to subscribe.
func (a *ACL) AllowSubscribe(clientID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.check(clientID, a.allowSubscribe, a.denySubscribe)
}

// check applies allow/deny logic.
// deny wins over allow.
// empty allow-list means "allow all" (unless explicitly denied).
func (a *ACL) check(clientID string, allow, deny map[string]struct{}) bool {
	if _, denied := deny[clientID]; denied {
		return false
	}
	if len(allow) == 0 {
		return true // empty allow-list = allow all
	}
	_, ok := allow[clientID]
	return ok
}
