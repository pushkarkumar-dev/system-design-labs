package rpc

// server.go — RPC server: registration, reflection-based dispatch, connection handling.
//
// v0: Register(service any) reflects on exported methods whose signatures match
//
//	func(ctx context.Context, req *Request) (*Response, error)
//
// and maps "ServiceName.MethodName" → handler. Each accepted TCP connection
// gets its own goroutine (ServerConn). Frames are read in a loop; the method
// name is extracted from the JSON payload and dispatched.
//
// v2: Use(middleware...) wraps the dispatch chain. Middleware is applied
// innermost-first so Use(A, B, C) calls A → B → C → handler.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"reflect"
	"sync"
)

// Request is the decoded inbound call from the client.
type Request struct {
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args"`
}

// Response is the outbound reply to the client.
type Response struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// HandlerFunc is the internal dispatch signature.
type HandlerFunc func(ctx context.Context, req *Request) (*Response, error)

// Server accepts connections and dispatches RPC calls.
type Server struct {
	mu          sync.RWMutex
	handlers    map[string]HandlerFunc // "Service.Method" → handler
	middlewares []Middleware
}

// NewServer creates an empty RPC server.
func NewServer() *Server {
	return &Server{
		handlers: make(map[string]HandlerFunc),
	}
}

// Register inspects svc using reflection and registers every exported method
// with the signature func(context.Context, *Request) (*Response, error).
// The handler is registered as "TypeName.MethodName".
func (s *Server) Register(svc interface{}) error {
	typ := reflect.TypeOf(svc)
	val := reflect.ValueOf(svc)
	name := typ.Name()
	if typ.Kind() == reflect.Ptr {
		name = typ.Elem().Name()
	}

	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	reqType := reflect.TypeOf((*Request)(nil))
	respType := reflect.TypeOf((*Response)(nil))
	errType := reflect.TypeOf((*error)(nil)).Elem()

	registered := 0
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		mt := m.Type

		// Expected: func(svc, ctx, *Request) (*Response, error)
		if mt.NumIn() != 3 || mt.NumOut() != 2 {
			continue
		}
		if !mt.In(1).Implements(ctxType) {
			continue
		}
		if mt.In(2) != reqType {
			continue
		}
		if mt.Out(0) != respType {
			continue
		}
		if !mt.Out(1).Implements(errType) {
			continue
		}

		method := val.Method(i)
		key := name + "." + m.Name
		s.mu.Lock()
		s.handlers[key] = func(ctx context.Context, req *Request) (*Response, error) {
			out := method.Call([]reflect.Value{
				reflect.ValueOf(ctx),
				reflect.ValueOf(req),
			})
			var resp *Response
			if !out[0].IsNil() {
				resp = out[0].Interface().(*Response)
			}
			var err error
			if !out[1].IsNil() {
				err = out[1].Interface().(error)
			}
			return resp, err
		}
		s.mu.Unlock()
		registered++
	}

	if registered == 0 {
		return fmt.Errorf("rpc: %s has no eligible methods (want func(context.Context, *Request) (*Response, error))", name)
	}
	return nil
}

// RegisterFunc registers a named handler function directly (bypasses reflection).
func (s *Server) RegisterFunc(name string, fn HandlerFunc) {
	s.mu.Lock()
	s.handlers[name] = fn
	s.mu.Unlock()
}

// Use appends middleware to the server's dispatch chain. Middleware is called
// in order: Use(A, B, C) results in A wrapping B wrapping C wrapping the handler.
func (s *Server) Use(mw ...Middleware) {
	s.middlewares = append(s.middlewares, mw...)
}

// Serve accepts connections from l and handles them in separate goroutines.
// Serve blocks until l is closed.
func (s *Server) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

// handleConn reads frames from conn, dispatches each call, and writes the
// response frame back. One goroutine per connection (ServerConn pattern).
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	var wrMu sync.Mutex

	for {
		frame, err := ReadFrame(reader)
		if err != nil {
			return // connection closed or read error — silently exit goroutine
		}

		// Streaming frames are handled by the stream subsystem (v1).
		if frame.Flags&FlagIsStream != 0 {
			s.handleStreamFrame(conn, writer, &wrMu, frame)
			continue
		}

		go func(f Frame) {
			resp := s.dispatch(context.Background(), f)
			wrMu.Lock()
			_ = WriteFrame(writer, resp)
			_ = writer.Flush()
			wrMu.Unlock()
		}(frame)
	}
}

// dispatch decodes the request, finds the handler, runs the middleware chain,
// and returns a response frame.
func (s *Server) dispatch(ctx context.Context, f Frame) Frame {
	var req Request
	if err := json.Unmarshal(f.Payload, &req); err != nil {
		return NewErrorFrame(f.RequestID, "invalid request JSON: "+err.Error())
	}

	s.mu.RLock()
	handler, ok := s.handlers[req.Method]
	s.mu.RUnlock()

	if !ok {
		return NewErrorFrame(f.RequestID, "unknown method: "+req.Method)
	}

	// Build the middleware chain (innermost first).
	h := handler
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		mw := s.middlewares[i]
		next := h
		h = func(ctx context.Context, req *Request) (*Response, error) {
			return mw(ctx, req.Method, next)
		}
	}

	resp, err := h(ctx, &req)
	if err != nil {
		return NewErrorFrame(f.RequestID, err.Error())
	}
	if resp == nil {
		resp = &Response{}
	}

	payload, merr := json.Marshal(resp)
	if merr != nil {
		log.Printf("rpc: marshal response: %v", merr)
		return NewErrorFrame(f.RequestID, "internal marshal error")
	}

	return NewFrame(f.RequestID, payload)
}
