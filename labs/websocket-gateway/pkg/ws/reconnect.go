package ws

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// tokenSecret is the HMAC-SHA256 signing key for reconnect tokens.
// In production this would be loaded from a secret store.
var tokenSecret = []byte("dev-secret-do-not-use-in-prod")

// ErrTokenExpired is returned when a reconnect token has passed its expiry time.
var ErrTokenExpired = errors.New("ws: reconnect token expired")

// ErrTokenInvalid is returned when the HMAC signature on a token is wrong.
var ErrTokenInvalid = errors.New("ws: reconnect token signature invalid")

// TokenClaims holds the data embedded in a reconnect token.
type TokenClaims struct {
	UserID  string    `json:"uid"`
	RoomID  string    `json:"rid"`
	Expiry  time.Time `json:"exp"`
	LastSeq uint64    `json:"seq"`
}

// TokenStore issues and verifies reconnect tokens.
// It intentionally does not persist tokens — on server restart all tokens
// expire (which is listed in "what the toy misses").
type TokenStore struct {
	mu     sync.Mutex
	issued map[string]struct{}
}

func newTokenStore() *TokenStore {
	return &TokenStore{issued: make(map[string]struct{})}
}

// Issue creates a signed reconnect token for the given claims.
// Tokens are valid for 5 minutes from the time of issue.
func (ts *TokenStore) Issue(userID, roomID string, lastSeq uint64) (string, error) {
	claims := TokenClaims{
		UserID:  userID,
		RoomID:  roomID,
		Expiry:  time.Now().Add(5 * time.Minute),
		LastSeq: lastSeq,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(encoded)
	token := encoded + "." + sig

	ts.mu.Lock()
	ts.issued[token] = struct{}{}
	ts.mu.Unlock()

	return token, nil
}

// Verify validates the token's HMAC signature and expiry.
func (ts *TokenStore) Verify(token string) (TokenClaims, error) {
	// Split at the last '.'.
	dot := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return TokenClaims{}, ErrTokenInvalid
	}

	encoded := token[:dot]
	sig := token[dot+1:]

	if sign(encoded) != sig {
		return TokenClaims{}, ErrTokenInvalid
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return TokenClaims{}, ErrTokenInvalid
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return TokenClaims{}, ErrTokenInvalid
	}

	if time.Now().After(claims.Expiry) {
		return TokenClaims{}, ErrTokenExpired
	}

	return claims, nil
}

func sign(data string) string {
	mac := hmac.New(sha256.New, tokenSecret)
	mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Message is a single timestamped message stored in the ring buffer.
type Message struct {
	SeqNo   uint64
	Payload []byte
}

// RingBuffer is a fixed-size circular buffer holding the last N messages
// for a room. It enables replay of missed messages on client reconnect.
type RingBuffer struct {
	mu      sync.RWMutex
	buf     []Message
	size    int
	head    int    // index of the oldest slot
	count   int    // number of valid entries (0..size)
	nextSeq atomic.Uint64
}

// newRingBuffer creates a ring buffer with the given capacity.
func newRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		buf:  make([]Message, size),
		size: size,
	}
}

// Append adds a message to the ring buffer, overwriting the oldest entry if
// the buffer is full. Returns the assigned sequence number.
func (rb *RingBuffer) Append(payload []byte) uint64 {
	seqno := rb.nextSeq.Add(1)

	rb.mu.Lock()
	defer rb.mu.Unlock()

	pos := (rb.head + rb.count) % rb.size
	if rb.count == rb.size {
		// Buffer full: overwrite oldest entry and advance head.
		rb.buf[rb.head] = Message{SeqNo: seqno, Payload: payload}
		rb.head = (rb.head + 1) % rb.size
	} else {
		rb.buf[pos] = Message{SeqNo: seqno, Payload: payload}
		rb.count++
	}
	return seqno
}

// Since returns all messages with SeqNo greater than afterSeq, in order.
// Returns at most rb.size messages (the full buffer contents if afterSeq
// predates the oldest retained message).
func (rb *RingBuffer) Since(afterSeq uint64) []Message {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	var result []Message
	for i := 0; i < rb.count; i++ {
		idx := (rb.head + i) % rb.size
		if rb.buf[idx].SeqNo > afterSeq {
			result = append(result, rb.buf[idx])
		}
	}
	return result
}

// Len returns the number of messages currently stored.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
