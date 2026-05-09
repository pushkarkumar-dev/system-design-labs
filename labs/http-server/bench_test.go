// Benchmarks for the HTTP/1.1 server.
//
// Run with:
//
//	go test -bench=. -benchmem -benchtime=5s ./...
//
// Typical results on an M2 MacBook Pro (estimated — run locally for real numbers):
//
//	BenchmarkV0HTTP10-10              85,000 req/sec   ~11,700 ns/op  (new conn per req)
//	BenchmarkV1KeepAlive-10          420,000 req/sec    ~2,380 ns/op  (persistent conn)
//	BenchmarkV1NewConnPerReq-10       12,000 req/sec   ~83,000 ns/op  (TCP handshake cost)
//	BenchmarkV2Pipeline10-10         ~8x over sequential on same connection
package bench_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"

	"dev.pushkar/http-server/pkg/httpserver"
)

// startBenchV1 starts an HTTP/1.1 server on a random port for benchmarks.
func startBenchV1(b *testing.B) string {
	b.Helper()
	srv := httpserver.NewV1("127.0.0.1:0")
	srv.Mux = httpserver.BuildDefaultMux()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				w := bufio.NewWriter(c)
				for {
					req, err := httpserver.ParseRequestExported(r)
					if err != nil {
						return
					}
					resp := srv.Mux.ServeHTTP(req)
					if resp == nil {
						resp = &httpserver.Response{Status: 404, Body: []byte("Not Found")}
					}
					keepAlive := httpserver.ShouldKeepAliveExported(req)
					httpserver.WriteResponseV1Exported(w, resp, keepAlive)
					if !keepAlive {
						return
					}
				}
			}(conn)
		}
	}()

	b.Cleanup(func() { ln.Close() })
	return addr
}

// drainResponse reads and discards one HTTP response from r.
// Returns false if the connection was closed (Connection: close).
func drainResponse(r *bufio.Reader) bool {
	statusLine, err := r.ReadString('\n')
	if err != nil {
		return false
	}
	_ = statusLine

	var contentLength int
	keepAlive := true
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return false
		}
		hline = strings.TrimRight(hline, "\r\n")
		if hline == "" {
			break
		}
		lower := strings.ToLower(hline)
		var cl int
		if _, err := fmt.Sscanf(lower, "content-length: %d", &cl); err == nil {
			contentLength = cl
		}
		if lower == "connection: close" {
			keepAlive = false
		}
	}
	if contentLength > 0 {
		buf := make([]byte, contentLength)
		r.Read(buf)
	}
	return keepAlive
}

// ── BenchmarkV1KeepAlive ─────────────────────────────────────────────────────
// Measures throughput with a persistent connection (no TCP handshake overhead).
// Target: ≥ 400k req/sec on M2.

func BenchmarkV1KeepAlive(b *testing.B) {
	addr := startBenchV1(b)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	req := []byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn.Write(req)
		if !drainResponse(r) {
			b.Fatal("connection closed unexpectedly")
		}
	}
}

// ── BenchmarkV1NewConnPerReq ─────────────────────────────────────────────────
// Measures throughput with a new TCP connection per request.
// Shows the cost of the TCP handshake (~0.3ms on loopback).
// Target: ~12k req/sec (shows the 35x penalty vs keep-alive).

func BenchmarkV1NewConnPerReq(b *testing.B) {
	addr := startBenchV1(b)

	req := []byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			b.Fatal(err)
		}
		conn.Write(req)
		r := bufio.NewReader(conn)
		drainResponse(r)
		conn.Close()
	}
}

// ── BenchmarkV2Pipeline10 ────────────────────────────────────────────────────
// Measures pipelining: sends 10 requests before reading any response.
// Compares with 10 sequential round-trips to show the pipelining benefit.

func BenchmarkV2Pipeline10(b *testing.B) {
	srv := httpserver.NewV2("127.0.0.1:0")
	srv.Mux = httpserver.BuildDefaultMux()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	srv.SetListener(ln)
	go srv.ServeListener()
	addr := ln.Addr().String()
	b.Cleanup(func() { srv.Shutdown() })

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	const pipelineN = 10
	req := []byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n")
	batch := make([]byte, 0, len(req)*pipelineN)
	for i := 0; i < pipelineN; i++ {
		batch = append(batch, req...)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Send all 10 requests at once (pipeline)
		conn.Write(batch)
		// Read all 10 responses
		for j := 0; j < pipelineN; j++ {
			if !drainResponse(r) {
				b.Fatal("connection closed mid-pipeline")
			}
		}
	}
}

// ── BenchmarkV2Sequential10 ──────────────────────────────────────────────────
// Sends 10 requests sequentially (send → wait → send → wait) on one connection.
// Baseline to compare against BenchmarkV2Pipeline10.

func BenchmarkV2Sequential10(b *testing.B) {
	srv := httpserver.NewV2("127.0.0.1:0")
	srv.Mux = httpserver.BuildDefaultMux()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	srv.SetListener(ln)
	go srv.ServeListener()
	addr := ln.Addr().String()
	b.Cleanup(func() { srv.Shutdown() })

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	req := []byte("GET / HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		for j := 0; j < 10; j++ {
			conn.Write(req)
			drainResponse(r)
		}
	}
}
