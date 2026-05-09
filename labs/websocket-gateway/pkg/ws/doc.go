// Package ws implements a WebSocket pub/sub gateway from scratch.
//
// The package is organized across three implementation stages:
//
//   - handshake.go  – HTTP upgrade, Sec-WebSocket-Accept key derivation (RFC 6455)
//   - frame.go      – WebSocket frame parse/encode, masking/unmasking
//   - client.go     – Client, readPump, writePump, per-client send buffer
//   - hub.go        – Hub, Room, broadcast, connection limit, atomic stats
//   - presence.go   – Presence events: joined/left broadcasts with member lists
//   - reconnect.go  – ReconnectToken (HMAC-SHA256), ring buffer, sequence numbers
//
// Each file corresponds to one identifiable concern. v0 uses only handshake.go,
// frame.go, and hub.go. v1 adds client.go and presence.go. v2 adds reconnect.go.
package ws
