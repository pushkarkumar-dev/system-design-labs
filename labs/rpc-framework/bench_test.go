// Benchmarks for the rpc-framework.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=3s ./...
//
// Estimated results on an M2 MacBook Pro:
//
//	BenchmarkFrameEncodeDecode-10     12,000,000 ops/sec   ~83 ns/op
//	BenchmarkUnaryRPC-10                  85,000 ops/sec  ~11764 ns/op
//	BenchmarkMiddlewareChain-10        4,500,000 ops/sec   ~222 ns/op
package bench_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/pushkar1005/system-design-labs/labs/rpc-framework/pkg/rpc"
)

// BenchmarkFrameEncodeDecode measures the cost of serialising + deserialising
// one frame. Target: ≥ 10,000,000 frames/sec (no allocs in the hot path).
func BenchmarkFrameEncodeDecode(b *testing.B) {
	payload := []byte(`{"method":"Math.Add","args":{"a":1,"b":2}}`)
	f := rpc.NewFrame(1, payload)

	var buf strings.Builder
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = rpc.WriteFrame(&buf, f)
		r := strings.NewReader(buf.String())
		_, _ = rpc.ReadFrame(r)
	}
}

// BenchmarkUnaryRPC measures end-to-end loopback RPC throughput.
// This includes: JSON marshal, TCP write, server dispatch, JSON unmarshal, reply.
func BenchmarkUnaryRPC(b *testing.B) {
	type AddReq struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	type AddResp struct {
		Sum int `json:"sum"`
	}

	srv := rpc.NewServer()
	srv.RegisterFunc("Math.Add", func(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
		var args AddReq
		_ = json.Unmarshal(req.Args, &args)
		return &rpc.Response{Result: AddResp{Sum: args.A + args.B}}, nil
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = srv.Serve(l) }()
	defer l.Close()

	client, err := rpc.Dial(l.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var reply AddResp
		if err := client.Call(context.Background(), "Math.Add", AddReq{A: i, B: i + 1}, &reply); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMiddlewareChain measures the overhead of running a 3-deep middleware
// chain without actually making a network call (server-side dispatch only).
func BenchmarkMiddlewareChain(b *testing.B) {
	registry := &rpc.MetricsRegistry{}

	srv := rpc.NewServer()
	srv.Use(
		rpc.RecoveryMiddleware,
		rpc.NewMetricsMiddleware(registry),
	)
	srv.RegisterFunc("Bench.Noop", func(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
		return &rpc.Response{Result: "ok"}, nil
	})

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	go func() { _ = srv.Serve(l) }()
	defer l.Close()

	client, err := rpc.Dial(l.Addr().String())
	if err != nil {
		b.Fatal(err)
	}
	defer client.Close()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := client.Call(context.Background(), "Bench.Noop", nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFrameWriterBuffered measures raw frame write throughput using a
// bufio.Writer — simulating the server-side write path in the hot path.
func BenchmarkFrameWriterBuffered(b *testing.B) {
	payload := make([]byte, 50) // ~50-byte JSON payload
	f := rpc.NewFrame(1, payload)

	// Write to /dev/null equivalent via a discard writer.
	w := bufio.NewWriter(discardWriter{})
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = rpc.WriteFrame(w, f)
		if w.Buffered() > 32*1024 {
			_ = w.Flush()
		}
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
