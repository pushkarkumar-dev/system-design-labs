// cmd/demo/main.go — calculator server + client demo for the rpc-framework lab.
//
// Run:
//
//	go run ./cmd/demo
//
// Expected output shows v0 (unary), v2 (middleware), and IDL parsing.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/rpc-framework/pkg/rpc"
)

// ── Calculator service ────────────────────────────────────────────────────────

type Calculator struct{}

type AddRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

type AddResponse struct {
	Sum int `json:"sum"`
}

func (c *Calculator) Add(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
	var args AddRequest
	if err := json.Unmarshal(req.Args, &args); err != nil {
		return nil, fmt.Errorf("decode args: %w", err)
	}
	log.Printf("Calculator.Add(%d, %d)", args.A, args.B)
	return &rpc.Response{Result: AddResponse{Sum: args.A + args.B}}, nil
}

func (c *Calculator) Multiply(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
	var args AddRequest
	if err := json.Unmarshal(req.Args, &args); err != nil {
		return nil, fmt.Errorf("decode args: %w", err)
	}
	log.Printf("Calculator.Multiply(%d, %d)", args.A, args.B)
	return &rpc.Response{Result: AddResponse{Sum: args.A * args.B}}, nil
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	fmt.Println("=== RPC Framework Demo ===")
	fmt.Println()

	// ── Part 1: v0 unary RPC ─────────────────────────────────────────────────
	fmt.Println("--- Part 1: Unary RPC (v0 binary framing) ---")

	registry := &rpc.MetricsRegistry{}

	srv := rpc.NewServer()
	srv.Use(
		rpc.LoggingMiddleware,
		rpc.RecoveryMiddleware,
		rpc.NewMetricsMiddleware(registry),
	)
	if err := srv.Register(&Calculator{}); err != nil {
		log.Fatalf("register Calculator: %v", err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	go func() {
		if err := srv.Serve(l); err != nil {
			// listener closed — normal shutdown
		}
	}()
	addr := l.Addr().String()
	fmt.Printf("Server listening on %s\n", addr)

	client, err := rpc.Dial(addr)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer client.Close()

	// Unary Add call.
	var addReply AddResponse
	if err := client.Call(context.Background(), "Calculator.Add", AddRequest{A: 7, B: 35}, &addReply); err != nil {
		log.Fatalf("Add: %v", err)
	}
	fmt.Printf("Calculator.Add(7, 35) = %d\n", addReply.Sum)

	// Unary Multiply call.
	var mulReply AddResponse
	if err := client.Call(context.Background(), "Calculator.Multiply", AddRequest{A: 6, B: 7}, &mulReply); err != nil {
		log.Fatalf("Multiply: %v", err)
	}
	fmt.Printf("Calculator.Multiply(6, 7) = %d\n", mulReply.Sum)
	fmt.Println()

	// ── Part 2: unknown method ────────────────────────────────────────────────
	fmt.Println("--- Part 2: Unknown method returns error ---")
	err = client.Call(context.Background(), "Calculator.Divide", AddRequest{A: 10, B: 2}, nil)
	if err != nil {
		fmt.Printf("Error (expected): %v\n", err)
	}
	fmt.Println()

	// ── Part 3: context deadline ──────────────────────────────────────────────
	fmt.Println("--- Part 3: Context deadline cancels call ---")
	srv.RegisterFunc("Slow.Method", func(ctx context.Context, req *rpc.Request) (*rpc.Response, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return &rpc.Response{Result: "too late"}, nil
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err = client.Call(ctx, "Slow.Method", nil, nil)
	fmt.Printf("Deadline error (expected): %v\n", err)
	fmt.Println()

	// ── Part 4: IDL parsing ───────────────────────────────────────────────────
	fmt.Println("--- Part 4: IDL parsing (v2) ---")
	idlSrc := `
service Calculator {
    method Add(AddRequest) returns AddResponse
    method Sum(stream AddRequest) returns AddResponse
}
`
	desc, err := rpc.ParseIDL(idlSrc)
	if err != nil {
		log.Fatalf("ParseIDL: %v", err)
	}
	fmt.Printf("Parsed service: %s\n", desc.Name)
	for _, m := range desc.Methods {
		stream := ""
		if m.ClientStream {
			stream = " [client-streaming]"
		}
		fmt.Printf("  method %s(%s) returns %s%s\n", m.Name, m.InputType, m.OutputType, stream)
	}
	fmt.Println()

	// ── Part 5: metrics ───────────────────────────────────────────────────────
	fmt.Println("--- Part 5: Metrics after all calls ---")
	// Allow async goroutines to settle.
	time.Sleep(100 * time.Millisecond)

	snap := registry.Snapshot()
	for method, s := range snap {
		fmt.Printf("  %s: calls=%d errors=%d\n", method, s.Calls, s.Errors)
	}
	fmt.Println()

	fmt.Println("=== Demo complete ===")
	l.Close()
	os.Exit(0)
}
