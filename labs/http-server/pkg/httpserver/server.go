// Package httpserver implements an HTTP/1.1 server from raw TCP sockets.
//
// Three progressive versions live in this file:
//
//   v0 — HTTP/1.0: accept TCP, parse request line + headers, send response.
//        One request per connection — TCP handshake cost every time.
//
//   v1 — HTTP/1.1 keep-alive: persistent connections, Content-Length framing,
//        a path router, and chunked transfer encoding for responses.
//
//   v2 — Pipelining, Slowloris mitigation (read/write deadlines), and
//        graceful shutdown via SIGTERM with a 30-second drain window.
//
// Key invariant across all versions: HTTP is a text protocol layered over TCP.
// Every "framework magic" is just string parsing and buffered I/O.

package httpserver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ── Shared types ─────────────────────────────────────────────────────────────

// Request holds a parsed HTTP request.
type Request struct {
	Method  string
	Path    string
	Version string            // "HTTP/1.0" or "HTTP/1.1"
	Headers map[string]string // header names are lower-cased
	Body    []byte
}

// Response holds an HTTP response to send.
type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
	Chunked bool // if true, send body using chunked transfer encoding
}

// HandlerFunc is a function that handles an HTTP request.
type HandlerFunc func(req *Request) *Response

// ── v0 — HTTP/1.0 over raw TCP ───────────────────────────────────────────────
//
// Lesson: HTTP/1.0 is just text. Parse one line for the method/path/version,
// read headers until a blank line, then write "HTTP/1.0 200 OK\r\n..." back.
// The server closes the connection after every response — no state to track.
// The visible cost: each request pays the full TCP handshake (~0.3ms loopback,
// ~50ms cross-datacenter). At 10,000 req/sec that is 3 seconds of pure overhead.

// ServerV0 is a minimal HTTP/1.0 server — one request per connection.
type ServerV0 struct {
	Addr    string
	handler HandlerFunc
}

// NewV0 creates an HTTP/1.0 server that calls handler for every GET request.
func NewV0(addr string, handler HandlerFunc) *ServerV0 {
	return &ServerV0{Addr: addr, handler: handler}
}

// ListenAndServe starts the server and blocks until the listener fails.
func (s *ServerV0) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleV0(conn)
	}
}

func (s *ServerV0) handleV0(conn net.Conn) {
	defer conn.Close()

	r := bufio.NewReader(conn)

	// Parse request line: METHOD SP Request-URI SP HTTP/version CRLF
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimRight(line, "\r\n")
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		writeResponseV0(conn, 400, "Bad Request")
		return
	}

	req := &Request{
		Method:  parts[0],
		Path:    parts[1],
		Version: parts[2],
		Headers: make(map[string]string),
	}

	// Read headers until blank line
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return
		}
		hline = strings.TrimRight(hline, "\r\n")
		if hline == "" {
			break
		}
		idx := strings.IndexByte(hline, ':')
		if idx < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(hline[:idx]))
		val := strings.TrimSpace(hline[idx+1:])
		req.Headers[key] = val
	}

	// HTTP/1.0 — only GET is supported in v0
	if req.Method != "GET" {
		writeResponseV0(conn, 405, "Method Not Allowed")
		return
	}

	resp := s.handler(req)
	if resp == nil {
		writeResponseV0(conn, 404, "Not Found")
		return
	}

	// Write response: HTTP/1.0 — connection closes after response automatically
	statusText := statusTexts[resp.Status]
	if statusText == "" {
		statusText = "Unknown"
	}
	fmt.Fprintf(conn, "HTTP/1.0 %d %s\r\n", resp.Status, statusText)
	fmt.Fprintf(conn, "Content-Length: %d\r\n", len(resp.Body))
	for k, v := range resp.Headers {
		fmt.Fprintf(conn, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(conn, "\r\n")
	conn.Write(resp.Body)
}

func writeResponseV0(conn net.Conn, status int, body string) {
	fmt.Fprintf(conn, "HTTP/1.0 %d %s\r\nContent-Length: %d\r\n\r\n%s",
		status, body, len(body), body)
}

// ── v1 — HTTP/1.1 keep-alive + chunked encoding + router ────────────────────
//
// Lesson: keep-alive means the TCP connection persists across multiple requests.
// This requires knowing exactly where each request ends. Content-Length is the
// only reliable delimiter for request bodies — without it, the parser would
// have to read until the connection closes (HTTP/1.0 behaviour). Chunked
// transfer encoding solves the symmetric problem for responses: when the
// server doesn't know the final body size before streaming begins, it sends
// hex-length prefixed chunks terminated by "0\r\n\r\n".

// Mux is a simple path-based router: map[string]HandlerFunc.
type Mux struct {
	routes map[string]HandlerFunc
}

// NewMux creates an empty router.
func NewMux() *Mux {
	return &Mux{routes: make(map[string]HandlerFunc)}
}

// Handle registers handler for the given path.
func (m *Mux) Handle(path string, h HandlerFunc) {
	m.routes[path] = h
}

// ServeHTTP dispatches the request to the registered handler or returns 404.
func (m *Mux) ServeHTTP(req *Request) *Response {
	h, ok := m.routes[req.Path]
	if !ok {
		return &Response{Status: 404, Body: []byte("Not Found")}
	}
	return h(req)
}

// ServerV1 is an HTTP/1.1 server with keep-alive and a Mux router.
type ServerV1 struct {
	Addr string
	Mux  *Mux
}

// NewV1 creates an HTTP/1.1 server.
func NewV1(addr string) *ServerV1 {
	return &ServerV1{Addr: addr, Mux: NewMux()}
}

// ListenAndServe starts the server and blocks.
func (s *ServerV1) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleV1(conn)
	}
}

func (s *ServerV1) handleV1(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)

	for {
		req, err := parseRequest(r)
		if err != nil {
			// Client closed connection or malformed request — either way, done.
			return
		}

		resp := s.Mux.ServeHTTP(req)
		if resp == nil {
			resp = &Response{Status: 404, Body: []byte("Not Found")}
		}

		keepAlive := shouldKeepAlive(req)
		writeResponseV1(w, resp, keepAlive)

		if !keepAlive {
			return
		}
	}
}

// parseRequest reads one complete HTTP request from r.
// It handles Content-Length bodies — the only reliable framing for keep-alive.
func parseRequest(r *bufio.Reader) (*Request, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, io.EOF
	}
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("bad request line: %q", line)
	}

	req := &Request{
		Method:  parts[0],
		Path:    parts[1],
		Version: parts[2],
		Headers: make(map[string]string),
	}

	// Read headers
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		hline = strings.TrimRight(hline, "\r\n")
		if hline == "" {
			break
		}
		idx := strings.IndexByte(hline, ':')
		if idx < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(hline[:idx]))
		val := strings.TrimSpace(hline[idx+1:])
		req.Headers[key] = val
	}

	// Read body if Content-Length is present
	if cls, ok := req.Headers["content-length"]; ok {
		n, err := strconv.Atoi(cls)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid content-length: %q", cls)
		}
		if n > 0 {
			req.Body = make([]byte, n)
			if _, err := io.ReadFull(r, req.Body); err != nil {
				return nil, err
			}
		}
	}

	return req, nil
}

// shouldKeepAlive returns true if the connection should remain open.
// HTTP/1.1 defaults to keep-alive; HTTP/1.0 defaults to close.
func shouldKeepAlive(req *Request) bool {
	conn := strings.ToLower(req.Headers["connection"])
	if conn == "close" {
		return false
	}
	if conn == "keep-alive" {
		return true
	}
	return req.Version == "HTTP/1.1"
}

// writeResponseV1 writes an HTTP/1.1 response with optional chunked encoding.
func writeResponseV1(w *bufio.Writer, resp *Response, keepAlive bool) {
	statusText := statusTexts[resp.Status]
	if statusText == "" {
		statusText = "Unknown"
	}

	fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", resp.Status, statusText)

	if keepAlive {
		fmt.Fprintf(w, "Connection: keep-alive\r\n")
	} else {
		fmt.Fprintf(w, "Connection: close\r\n")
	}

	// Emit headers
	for k, v := range resp.Headers {
		fmt.Fprintf(w, "%s: %s\r\n", k, v)
	}

	if resp.Chunked {
		// Chunked transfer encoding: hex-length CRLF chunk CRLF ... 0 CRLF CRLF
		// Use when Content-Length is unknown before streaming starts.
		fmt.Fprintf(w, "Transfer-Encoding: chunked\r\n\r\n")

		// For simplicity, send the body as a single chunk
		if len(resp.Body) > 0 {
			fmt.Fprintf(w, "%x\r\n", len(resp.Body))
			w.Write(resp.Body)
			fmt.Fprintf(w, "\r\n")
		}
		// Terminator: zero-length chunk
		fmt.Fprintf(w, "0\r\n\r\n")
	} else {
		fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(resp.Body))
		w.Write(resp.Body)
	}

	w.Flush()
}

// ── v2 — Pipelining, Slowloris mitigation, graceful shutdown ─────────────────
//
// HTTP/1.1 pipelining: the client sends N requests without waiting for
// responses. The server must respond in order (responses are ordered).
// A 10-request pipeline hits ~8× the throughput of sequential requests on
// the same connection because the RTT of requests 2–10 overlaps with the
// processing of request 1.
//
// Slowloris mitigation: set read/write deadlines on every connection. A
// Slowloris attacker holds connections open by sending 1 byte every 10
// seconds. With a 5-second read deadline the connection is closed before
// the next byte arrives. No configuration needed — just net.Conn.SetDeadline.
//
// Graceful shutdown: on SIGTERM, stop accepting new connections, drain
// in-flight requests with a 30-second timeout, then exit cleanly.

const (
	readDeadline  = 5 * time.Second  // kills Slowloris
	writeDeadline = 10 * time.Second // prevents slow receivers from holding the goroutine
	shutdownGrace = 30 * time.Second // drain window on SIGTERM
)

// ServerV2 is an HTTP/1.1 server with pipelining, deadlines, and graceful shutdown.
type ServerV2 struct {
	Addr     string
	Mux      *Mux
	listener net.Listener
	wg       sync.WaitGroup
	mu       sync.Mutex
	shutdown bool
}

// NewV2 creates an HTTP/1.1 server with Slowloris mitigation.
func NewV2(addr string) *ServerV2 {
	return &ServerV2{Addr: addr, Mux: NewMux()}
}

// ListenAndServe starts the server. Returns when the server shuts down.
func (s *ServerV2) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.listener = ln

	// Watch for SIGTERM/SIGINT in a goroutine
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		s.Shutdown()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			sd := s.shutdown
			s.mu.Unlock()
			if sd {
				// Listener was closed by Shutdown — wait for in-flight requests
				done := make(chan struct{})
				go func() { s.wg.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(shutdownGrace):
					log.Println("graceful shutdown timed out; forcing exit")
				}
				return nil
			}
			return err
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleV2(conn)
		}()
	}
}

// Shutdown stops accepting new connections and drains in-flight requests.
func (s *ServerV2) Shutdown() {
	s.mu.Lock()
	s.shutdown = true
	s.mu.Unlock()
	if s.listener != nil {
		s.listener.Close()
	}
}

// handleV2 serves one connection with pipelining and deadlines.
// Pipelining: the client may send N requests before reading any response.
// We parse them as fast as they arrive into reqCh, then the responder
// goroutine writes responses in order. Both goroutines share the net.Conn.
func (s *ServerV2) handleV2(conn net.Conn) {
	defer conn.Close()

	const pipelineDepth = 16
	type workItem struct {
		req  *Request
		resp chan *Response
	}

	reqCh := make(chan workItem, pipelineDepth)
	var wg sync.WaitGroup

	// Responder: writes responses in the order requests arrived.
	wg.Add(1)
	go func() {
		defer wg.Done()
		bw := bufio.NewWriter(conn)
		for item := range reqCh {
			resp := <-item.resp
			if resp == nil {
				resp = &Response{Status: 404, Body: []byte("Not Found")}
			}
			keepAlive := item.req != nil && shouldKeepAlive(item.req)
			conn.SetWriteDeadline(time.Now().Add(writeDeadline))
			writeResponseV1(bw, resp, keepAlive)
		}
	}()

	br := bufio.NewReader(conn)
	for {
		// Read deadline kills Slowloris: client must send the full request
		// line + headers within readDeadline seconds.
		conn.SetReadDeadline(time.Now().Add(readDeadline))

		req, err := parseRequest(br)
		if err != nil {
			break
		}

		respCh := make(chan *Response, 1)
		reqCh <- workItem{req: req, resp: respCh}

		// Dispatch handler immediately (non-blocking from the reader's perspective).
		go func(r *Request, ch chan *Response) {
			ch <- s.Mux.ServeHTTP(r)
		}(req, respCh)

		if !shouldKeepAlive(req) {
			break
		}
	}

	close(reqCh)
	wg.Wait()
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// statusTexts maps HTTP status codes to their canonical reason phrases.
var statusTexts = map[int]string{
	200: "OK",
	201: "Created",
	204: "No Content",
	400: "Bad Request",
	404: "Not Found",
	405: "Method Not Allowed",
	500: "Internal Server Error",
}

// decodeChunked reads a chunked-encoded body from r and returns the assembled bytes.
// Wire format: hex-length CRLF chunk CRLF ... 0 CRLF CRLF
func decodeChunked(r *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		sizeLine, err := r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("chunked: read size: %w", err)
		}
		sizeLine = strings.TrimRight(sizeLine, "\r\n")
		// Ignore chunk extensions (semicolon-separated after the size)
		if semi := strings.IndexByte(sizeLine, ';'); semi >= 0 {
			sizeLine = sizeLine[:semi]
		}
		n, err := strconv.ParseInt(strings.TrimSpace(sizeLine), 16, 64)
		if err != nil {
			return nil, fmt.Errorf("chunked: bad size %q: %w", sizeLine, err)
		}
		if n == 0 {
			// Terminator: consume the trailing CRLF
			r.ReadString('\n')
			break
		}
		chunk := make([]byte, n)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return nil, fmt.Errorf("chunked: read chunk: %w", err)
		}
		// Consume the CRLF after the chunk data
		r.ReadString('\n')
		out = append(out, chunk...)
	}
	return out, nil
}

// ── Built-in handlers ────────────────────────────────────────────────────────

// HelloHandler returns a simple greeting. Demonstrates v0/v1 GET handling.
func HelloHandler(req *Request) *Response {
	return &Response{
		Status: 200,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
		Body: []byte("Hello from your hand-rolled HTTP server!\n"),
	}
}

// EchoHandler returns the request headers as the response body.
func EchoHandler(req *Request) *Response {
	var sb strings.Builder
	sb.WriteString(req.Method + " " + req.Path + " " + req.Version + "\n")
	for k, v := range req.Headers {
		sb.WriteString(k + ": " + v + "\n")
	}
	return &Response{
		Status: 200,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
		Body: []byte(sb.String()),
	}
}

// UppercaseHandler reads a POST body and returns it uppercased.
func UppercaseHandler(req *Request) *Response {
	if req.Method != "POST" {
		return &Response{Status: 405, Body: []byte("Method Not Allowed")}
	}
	return &Response{
		Status: 200,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
		Body: []byte(strings.ToUpper(string(req.Body))),
	}
}

// ChunkedHelloHandler demonstrates chunked transfer encoding.
// The response body is sent as a single chunk but signalled via Transfer-Encoding.
func ChunkedHelloHandler(req *Request) *Response {
	return &Response{
		Status: 200,
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
		Body:    []byte("hello from chunked encoding\n"),
		Chunked: true,
	}
}

// BuildDefaultMux creates a Mux with the standard demo routes.
func BuildDefaultMux() *Mux {
	mux := NewMux()
	mux.Handle("/", HelloHandler)
	mux.Handle("/echo", EchoHandler)
	mux.Handle("/uppercase", UppercaseHandler)
	mux.Handle("/chunked", ChunkedHelloHandler)
	return mux
}

// DecodeChunked is the exported wrapper for tests and integration use.
var DecodeChunked = decodeChunked

// ParseRequestExported exposes parseRequest for tests.
func ParseRequestExported(r *bufio.Reader) (*Request, error) {
	return parseRequest(r)
}

// ShouldKeepAliveExported exposes shouldKeepAlive for tests.
func ShouldKeepAliveExported(req *Request) bool {
	return shouldKeepAlive(req)
}

// WriteResponseV1Exported exposes writeResponseV1 for tests.
func WriteResponseV1Exported(w *bufio.Writer, resp *Response, keepAlive bool) {
	writeResponseV1(w, resp, keepAlive)
}

// SetListener allows tests to inject a pre-bound listener so ServerV2 can use
// a random port chosen by the OS.
func (s *ServerV2) SetListener(ln net.Listener) {
	s.listener = ln
}

// ServeListener accepts connections on the already-set listener.
// Use after SetListener to start serving without re-binding.
func (s *ServerV2) ServeListener() error {
	if s.listener == nil {
		return fmt.Errorf("no listener set — call SetListener first")
	}
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			sd := s.shutdown
			s.mu.Unlock()
			if sd {
				done := make(chan struct{})
				go func() { s.wg.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(shutdownGrace):
					log.Println("graceful shutdown timed out")
				}
				return nil
			}
			return err
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleV2(conn)
		}()
	}
}

// Ensure errors package is used
var _ = errors.New

// Ensure context package is used (used by shutdown pattern)
var _ = context.Background
