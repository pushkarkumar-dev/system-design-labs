// Command server runs the FaaS runtime HTTP server with a few demo functions.
//
// Usage:
//
//	go run ./cmd/server        # starts on :8080
//	go run ./cmd/server -addr :9090
//
// Demo endpoints:
//
//	GET  /functions          → list registered functions
//	POST /invoke/echo        → echo body back
//	POST /invoke/upper       → uppercase the body
//	POST /invoke/slow        → sleeps 200ms (demos timeout behaviour)
//	GET  /stats              → invocation statistics
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/faas-runtime/pkg/faas"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	// v2 runtime: warm pool + snapshots + billing.
	rt := faas.NewRuntimeFull(3)

	// echo: returns the request body verbatim.
	rt.RegisterFull("echo", func(ctx context.Context, req faas.Request) faas.Response {
		return faas.Response{
			StatusCode: http.StatusOK,
			Body:       req.Body,
			Headers:    map[string]string{"Content-Type": "text/plain"},
		}
	}, 10*time.Second, 128)

	// upper: returns the body uppercased.
	rt.RegisterFull("upper", func(ctx context.Context, req faas.Request) faas.Response {
		return faas.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(strings.ToUpper(string(req.Body))),
			Headers:    map[string]string{"Content-Type": "text/plain"},
		}
	}, 10*time.Second, 128)

	// slow: sleeps 200ms — use with a short timeout to trigger 504.
	rt.RegisterFull("slow", func(ctx context.Context, req faas.Request) faas.Response {
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
		}
		return faas.Response{StatusCode: http.StatusOK, Body: []byte("done")}
	}, 500*time.Millisecond, 256)

	// greet: reads a "name" query parameter.
	rt.RegisterFull("greet", func(ctx context.Context, req faas.Request) faas.Response {
		name := req.QueryParams["name"]
		if name == "" {
			name = "world"
		}
		return faas.Response{
			StatusCode: http.StatusOK,
			Body:       []byte(fmt.Sprintf("Hello, %s!", name)),
			Headers:    map[string]string{"Content-Type": "text/plain"},
		}
	}, 10*time.Second, 128)

	log.Printf("FaaS runtime listening on %s", *addr)
	log.Printf("Try: curl -X POST http://localhost%s/invoke/echo -d 'hello'", *addr)
	log.Printf("Try: curl http://localhost%s/functions", *addr)
	log.Fatal(http.ListenAndServe(*addr, rt))
}
