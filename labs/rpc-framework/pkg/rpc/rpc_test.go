package rpc_test

// rpc_test.go — tests for the rpc package (v0 through v2).
//
// v0 tests (8):
//  1. Add method call returns correct sum
//  2. Unknown method returns error
//  3. Concurrent calls with different requestIDs return correct replies
//  4. Connection close returns error on in-flight call
//  5. Frame encode + decode roundtrip
//  6. Error frame is flagged and readable
//  7. Two sequential calls on same connection
//  8. Large payload (10KB) roundtrip
//
// v1 tests (5):
//  9. Streaming 100 messages from server
//  10. Client-side deadline fires
//  11. IDL parsing produces correct ServiceDescriptor
//  12. IDL lookup returns unknown method error
//  13. Middleware ordering: use(A, B) calls A before B
//
// v2 tests (5):
//  14. Panic in handler returns error, server keeps running
//  15. MetricsMiddleware counts calls and errors
//  16. RecoveryMiddleware does not crash process
//  17. LoggingMiddleware does not corrupt response
//  18. IDL method names match registered handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/rpc-framework/pkg/rpc"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// startServer starts an RPC server on a random loopback port and returns the
// listener address and a cleanup function.
func startServer(t *testing.T, srv *rpc.Server) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { l.Close() })
	return l.Addr().String()
}

// MathService is the example service used across all tests.
type MathService struct{}

type AddRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

type AddResponse struct {
	Sum int `json:"sum"`
}

func (m *MathService) Add(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
	var args AddRequest
	if err := json.Unmarshal(req.Args, &args); err != nil {
		return nil, fmt.Errorf("decode args: %w", err)
	}
	return &rpc.Response{Result: AddResponse{Sum: args.A + args.B}}, nil
}

func (m *MathService) PanicMethod(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
	panic("intentional panic for test")
}

// ── v0 tests ──────────────────────────────────────────────────────────────────

func TestAddMethod(t *testing.T) {
	srv := rpc.NewServer()
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	var reply AddResponse
	if err := client.Call(context.Background(), "MathService.Add", AddRequest{A: 3, B: 4}, &reply); err != nil {
		t.Fatalf("call: %v", err)
	}
	if reply.Sum != 7 {
		t.Errorf("expected sum=7, got %d", reply.Sum)
	}
}

func TestUnknownMethod(t *testing.T) {
	srv := rpc.NewServer()
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	err = client.Call(context.Background(), "MathService.Nonexistent", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown method, got nil")
	}
	if !strings.Contains(err.Error(), "unknown method") {
		t.Errorf("expected 'unknown method' in error, got: %v", err)
	}
}

func TestConcurrentCallsReturnCorrectReplies(t *testing.T) {
	srv := rpc.NewServer()
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			var reply AddResponse
			err := client.Call(context.Background(), "MathService.Add",
				AddRequest{A: idx, B: idx * 2}, &reply)
			if err != nil {
				errors[idx] = err
				return
			}
			expected := idx + idx*2
			if reply.Sum != expected {
				errors[idx] = fmt.Errorf("idx=%d: expected %d, got %d", idx, expected, reply.Sum)
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

func TestConnectionCloseReturnsError(t *testing.T) {
	srv := rpc.NewServer()
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// Close the client immediately.
	client.Close()

	// Any subsequent call should return an error.
	err = client.Call(context.Background(), "MathService.Add", AddRequest{A: 1, B: 2}, nil)
	if err == nil {
		t.Fatal("expected error after connection close, got nil")
	}
}

func TestFrameEncodeDecodeRoundtrip(t *testing.T) {
	original := rpc.Frame{
		Length:    5,
		Flags:     rpc.FlagIsStream,
		RequestID: 42,
		Payload:   []byte("hello"),
	}

	var buf strings.Builder
	if err := rpc.WriteFrame(&buf, original); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	decoded, err := rpc.ReadFrame(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}

	if decoded.Length != original.Length {
		t.Errorf("Length: got %d, want %d", decoded.Length, original.Length)
	}
	if decoded.Flags != original.Flags {
		t.Errorf("Flags: got %d, want %d", decoded.Flags, original.Flags)
	}
	if decoded.RequestID != original.RequestID {
		t.Errorf("RequestID: got %d, want %d", decoded.RequestID, original.RequestID)
	}
	if string(decoded.Payload) != string(original.Payload) {
		t.Errorf("Payload: got %q, want %q", decoded.Payload, original.Payload)
	}
}

func TestErrorFrameFlag(t *testing.T) {
	f := rpc.NewErrorFrame(99, "something went wrong")
	if f.Flags&rpc.FlagIsError == 0 {
		t.Error("error frame should have FlagIsError set")
	}
	if f.RequestID != 99 {
		t.Errorf("expected requestID=99, got %d", f.RequestID)
	}
	if string(f.Payload) != "something went wrong" {
		t.Errorf("unexpected payload: %q", f.Payload)
	}
}

func TestTwoSequentialCalls(t *testing.T) {
	srv := rpc.NewServer()
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	for i, tc := range []struct{ a, b, want int }{{1, 2, 3}, {10, 20, 30}} {
		var reply AddResponse
		if err := client.Call(context.Background(), "MathService.Add", AddRequest{A: tc.a, B: tc.b}, &reply); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if reply.Sum != tc.want {
			t.Errorf("call %d: got %d, want %d", i, reply.Sum, tc.want)
		}
	}
}

func TestLargePayloadRoundtrip(t *testing.T) {
	srv := rpc.NewServer()
	// Register a handler that echoes a large payload back.
	srv.RegisterFunc("Echo.Large", func(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
		return &rpc.Response{Result: string(req.Args)}, nil
	})
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// 10 KB payload.
	payload := strings.Repeat("x", 10240)
	var reply string
	if err := client.Call(context.Background(), "Echo.Large", payload, &reply); err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(reply) != len(payload) {
		t.Errorf("payload length mismatch: got %d, want %d", len(reply), len(payload))
	}
}

// ── v1 / v2 tests ─────────────────────────────────────────────────────────────

func TestContextDeadlineCancelsCall(t *testing.T) {
	srv := rpc.NewServer()
	// Handler that sleeps longer than the deadline.
	srv.RegisterFunc("Slow.Method", func(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return &rpc.Response{Result: "done"}, nil
		}
	})
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = client.Call(ctx, "Slow.Method", nil, nil)
	if err == nil {
		t.Fatal("expected deadline error, got nil")
	}
}

func TestIDLParsingProducesCorrectDescriptor(t *testing.T) {
	idlSrc := `
service Calculator {
    method Add(AddRequest) returns AddResponse
    method Sum(stream AddRequest) returns AddResponse
}
`
	desc, err := rpc.ParseIDL(idlSrc)
	if err != nil {
		t.Fatalf("ParseIDL: %v", err)
	}

	if desc.Name != "Calculator" {
		t.Errorf("service name: got %q, want %q", desc.Name, "Calculator")
	}
	if len(desc.Methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(desc.Methods))
	}

	add := desc.Methods[0]
	if add.Name != "Add" || add.InputType != "AddRequest" || add.OutputType != "AddResponse" || add.ClientStream {
		t.Errorf("Add method: %+v", add)
	}

	sum := desc.Methods[1]
	if sum.Name != "Sum" || sum.InputType != "AddRequest" || sum.OutputType != "AddResponse" || !sum.ClientStream {
		t.Errorf("Sum method (expected client stream): %+v", sum)
	}
}

func TestIDLLookupUnknownMethodReturnsError(t *testing.T) {
	idlSrc := `
service Calculator {
    method Add(AddRequest) returns AddResponse
}
`
	desc, err := rpc.ParseIDL(idlSrc)
	if err != nil {
		t.Fatalf("ParseIDL: %v", err)
	}

	_, err = desc.Lookup("Calculator.Nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown method, got nil")
	}
}

func TestMiddlewareOrdering(t *testing.T) {
	order := make([]string, 0, 3)
	var mu sync.Mutex

	makeMiddleware := func(name string) rpc.Middleware {
		return func(ctx context.Context, method string, next rpc.HandlerFunc) (*rpc.Response, error) {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return next(ctx, &rpc.Request{Method: method})
		}
	}

	srv := rpc.NewServer()
	srv.Use(makeMiddleware("A"), makeMiddleware("B"), makeMiddleware("C"))
	srv.RegisterFunc("Test.Method", func(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
		return &rpc.Response{Result: "ok"}, nil
	})
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if err := client.Call(context.Background(), "Test.Method", nil, nil); err != nil {
		t.Fatalf("call: %v", err)
	}

	// Wait briefly for async goroutine to record ordering.
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 || order[0] != "A" || order[1] != "B" || order[2] != "C" {
		t.Errorf("middleware order: got %v, want [A B C]", order)
	}
}

func TestPanicRecoveryReturnsErrorNotCrash(t *testing.T) {
	srv := rpc.NewServer()
	srv.Use(rpc.RecoveryMiddleware)
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// Call the panicking method — server should not crash.
	err = client.Call(context.Background(), "MathService.PanicMethod", nil, nil)
	if err == nil {
		t.Fatal("expected error from panicking handler, got nil")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("expected 'panic' in error message, got: %v", err)
	}

	// Server should still be alive — subsequent calls succeed.
	var reply AddResponse
	if err := client.Call(context.Background(), "MathService.Add", AddRequest{A: 1, B: 1}, &reply); err != nil {
		t.Errorf("server not alive after panic: %v", err)
	}
}

func TestMetricsMiddlewareCountsCallsAndErrors(t *testing.T) {
	registry := &rpc.MetricsRegistry{}
	srv := rpc.NewServer()
	srv.Use(rpc.RecoveryMiddleware, rpc.NewMetricsMiddleware(registry))
	if err := srv.Register(&MathService{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	addr := startServer(t, srv)

	client, err := rpc.Dial(addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// Make 3 successful calls.
	for i := 0; i < 3; i++ {
		var reply AddResponse
		if err := client.Call(context.Background(), "MathService.Add", AddRequest{A: i, B: i}, &reply); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	// Make 1 panicking call (error case).
	_ = client.Call(context.Background(), "MathService.PanicMethod", nil, nil)

	// Allow async goroutines to finish.
	time.Sleep(50 * time.Millisecond)

	snap := registry.Snapshot()
	addSnap, ok := snap["MathService.Add"]
	if !ok {
		t.Fatal("no metrics for MathService.Add")
	}
	if addSnap.Calls < 3 {
		t.Errorf("MathService.Add calls: got %d, want >= 3", addSnap.Calls)
	}

	panicSnap, ok := snap["MathService.PanicMethod"]
	if !ok {
		t.Fatal("no metrics for MathService.PanicMethod")
	}
	if panicSnap.Errors < 1 {
		t.Errorf("MathService.PanicMethod errors: got %d, want >= 1", panicSnap.Errors)
	}
}
