// Package rpc implements a toy RPC framework in three progressive stages.
//
// # Stage v0 — Binary Framing + Request-Response
//
// Wire format: [4-byte length][1-byte flags][4-byte requestID][N-byte payload]
// where payload is JSON. The Client auto-increments a requestID and blocks
// waiting for the matching response frame. The Server reflects on registered
// service methods of the form func(ctx, *Request) (*Response, error) and
// dispatches by the method name embedded in the JSON payload.
//
// # Stage v1 — Streaming + Deadlines
//
// The flags byte gains semantics: bit 0 = IsStream, bit 1 = EndOfStream,
// bit 2 = IsError. ServerStream allows the server to send multiple frames
// back to the client for a single call. ClientStream allows the client to
// send multiple frames before the server responds. BidiStream combines both.
// Context deadlines are propagated across the wire.
//
// # Stage v2 — IDL Codegen + Middleware
//
// A simple .rpc IDL is parsed at startup to produce ServiceDescriptors.
// Middleware (logging, recovery, metrics) wraps each handler dispatch via
// Server.Use(). RecoveryMiddleware catches handler panics and returns an
// error frame instead of crashing the server.
package rpc
