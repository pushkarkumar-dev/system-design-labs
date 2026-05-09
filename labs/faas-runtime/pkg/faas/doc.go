// Package faas implements a minimal Function-as-a-Service runtime.
//
// Three progressive stages live across the files in this package:
//
//	function.go  — shared Function, Request, Response types
//	runtime.go   — Runtime: Register, Invoke, HTTP server (v0)
//	pool.go      — InstancePool: warm pool, cold start, eviction (v1)
//	snapshot.go  — SnapshotStore: snapshot-based cold start acceleration (v2)
//	billing.go   — BillingRecord, BillingAggregator: per-invocation cost (v2)
//
// Stage overview:
//
//	v0 — Function registry + in-process execution. Per-invocation goroutine with
//	     context.WithTimeout. Handler panic is recovered and returns 500. HTTP
//	     server exposes POST /invoke/{name} and GET /functions.
//
//	v1 — Warm pool + cold start simulation. Each function has a pool of up to
//	     maxWarm idle instances. Acquiring a warm instance is immediate; cold start
//	     costs 50ms. A background eviction goroutine removes instances idle longer
//	     than warmTimeout. Invocation stats track cold/warm/timeout/panic counts.
//
//	v2 — Snapshotting + billing. After a cold start, the function's init state is
//	     snapshotted. Restoring from snapshot costs 5ms instead of 50ms — 10×
//	     faster. Billing charges max(1ms, ceiling(duration,1ms)) × (memMB/1024) ×
//	     $0.0000166667 per GB-ms.
package faas
