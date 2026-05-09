package bench_test

import (
	"bytes"
	"testing"

	ws "github.com/pushkar1005/system-design-labs/labs/websocket-gateway/pkg/ws"
)

// BenchmarkAcceptKey measures the SHA-1 + base64 accept-key derivation.
// At ~1.2M ops/sec this is never the gateway bottleneck.
func BenchmarkAcceptKey(b *testing.B) {
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ws.AcceptKey(key)
	}
}

// BenchmarkFrameEncode measures text frame encoding (no masking).
// Server-to-client frames are never masked per RFC 6455 §5.1.
func BenchmarkFrameEncode(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 128)
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = ws.WriteFrame(&buf, ws.OpcodeText, payload)
	}
}

// BenchmarkFrameDecode measures text frame decoding (server reads unmasked).
func BenchmarkFrameDecode(b *testing.B) {
	var frameBuf bytes.Buffer
	payload := bytes.Repeat([]byte("y"), 128)
	_ = ws.WriteFrame(&frameBuf, ws.OpcodeText, payload)
	frameBytes := frameBuf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(frameBytes)
		_, _ = ws.ReadFrame(r)
	}
}

// BenchmarkHubBroadcast100 measures the throughput of broadcasting one message
// to 100 clients via buffered channels. This is the core hot path.
func BenchmarkHubBroadcast100(b *testing.B) {
	hub := ws.NewHub(200)

	for i := 0; i < 100; i++ {
		c := ws.NewTestClient(i, hub)
		_ = hub.Register(c)
		hub.JoinRoom(c, "bench-room")
		go func(cl *ws.Client) {
			for range cl.SendBuf() {
			}
		}(c)
	}

	msg := []byte(`{"event":"message","room":"bench-room","content":"benchmark"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hub.Broadcast("bench-room", msg)
	}
}
