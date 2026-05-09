// Command server starts the HTTP/1.1 demo server.
//
// Usage:
//
//	go run ./cmd/server          # starts on :8080 with v2 (pipelining + graceful shutdown)
//	go run ./cmd/server -version=0  # HTTP/1.0 server
//	go run ./cmd/server -version=1  # HTTP/1.1 keep-alive server (no pipelining)
//	go run ./cmd/server -addr=:9090 # custom port
//
// Routes:
//
//	GET  /            → "Hello from your hand-rolled HTTP server!"
//	GET  /echo        → echoes all request headers back as plain text
//	POST /uppercase   → returns the request body uppercased
//	GET  /chunked     → demonstrates chunked transfer encoding
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"dev.pushkar/http-server/pkg/httpserver"
)

func main() {
	addr := flag.String("addr", ":8080", "TCP address to listen on")
	version := flag.Int("version", 2, "Server version: 0 (HTTP/1.0), 1 (HTTP/1.1), 2 (HTTP/1.1 + pipelining + graceful shutdown)")
	flag.Parse()

	switch *version {
	case 0:
		log.Printf("Starting HTTP/1.0 server on %s (one request per connection)\n", *addr)
		srv := httpserver.NewV0(*addr, httpserver.HelloHandler)
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}

	case 1:
		log.Printf("Starting HTTP/1.1 server on %s (keep-alive, router)\n", *addr)
		srv := httpserver.NewV1(*addr)
		srv.Mux = httpserver.BuildDefaultMux()
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}

	case 2:
		log.Printf("Starting HTTP/1.1 server on %s (pipelining, deadlines, graceful shutdown)\n", *addr)
		srv := httpserver.NewV2(*addr)
		srv.Mux = httpserver.BuildDefaultMux()
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown version: %d (use 0, 1, or 2)\n", *version)
		os.Exit(1)
	}
}
