package ws

import (
	"crypto/sha1" //nolint:gosec // RFC 6455 mandates SHA-1 for the accept key
	"encoding/base64"
	"errors"
	"net/http"
)

// magicGUID is the fixed GUID defined in RFC 6455 §1.3.
// It is concatenated with the client's Sec-WebSocket-Key and SHA-1-hashed
// to produce the Sec-WebSocket-Accept response header.
const magicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ErrNotWebSocket is returned when the HTTP request is not a valid WebSocket
// upgrade request.
var ErrNotWebSocket = errors.New("ws: not a valid WebSocket upgrade request")

// AcceptKey derives the Sec-WebSocket-Accept value from the client-supplied
// Sec-WebSocket-Key.
//
// From RFC 6455 §1.3:
//
//	accept = base64(SHA-1(key + magicGUID))
//
// This is the only use of SHA-1 in the protocol; it is not used for security
// but for a basic sanity check that the server understands WebSockets.
func AcceptKey(clientKey string) string {
	h := sha1.New() //nolint:gosec
	h.Write([]byte(clientKey))
	h.Write([]byte(magicGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// ValidateUpgrade checks that r is a valid WebSocket upgrade request and returns
// the Sec-WebSocket-Key. Returns ErrNotWebSocket if any required header is missing
// or has the wrong value.
func ValidateUpgrade(r *http.Request) (key string, err error) {
	if r.Method != http.MethodGet {
		return "", ErrNotWebSocket
	}
	if r.Header.Get("Upgrade") != "websocket" {
		return "", ErrNotWebSocket
	}
	if r.Header.Get("Connection") == "" {
		return "", ErrNotWebSocket
	}
	key = r.Header.Get("Sec-Websocket-Key")
	if key == "" {
		key = r.Header.Get("Sec-WebSocket-Key")
	}
	if key == "" {
		return "", ErrNotWebSocket
	}
	if r.Header.Get("Sec-Websocket-Version") != "13" && r.Header.Get("Sec-WebSocket-Version") != "13" {
		return "", ErrNotWebSocket
	}
	return key, nil
}

// DoHandshake performs the full HTTP 101 Switching Protocols handshake.
// On success, the underlying TCP connection is now speaking the WebSocket
// framing protocol and the caller must not touch w again through net/http.
func DoHandshake(w http.ResponseWriter, r *http.Request) (key string, err error) {
	key, err = ValidateUpgrade(r)
	if err != nil {
		http.Error(w, "not a websocket upgrade", http.StatusBadRequest)
		return "", err
	}

	w.Header().Set("Upgrade", "websocket")
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Sec-WebSocket-Accept", AcceptKey(key))
	w.WriteHeader(http.StatusSwitchingProtocols)
	return key, nil
}
