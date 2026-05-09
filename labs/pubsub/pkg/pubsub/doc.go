// Package pubsub implements a pub/sub broker in three progressive stages.
//
// v0 — Synchronous fan-out broker (~200 LoC):
//   - Topic holds a set of named Subscriptions
//   - Publish fans out synchronously: blocks until all subscribers receive
//   - MessageFilter: func(Message) bool — subscribers filter by attribute
//   - Key lesson: synchronous fan-out couples publisher latency to subscriber count
//
// v1 — Async delivery + retry + DLQ (~350 LoC):
//   - Each Subscription has a buffered channel (capacity 1000) + background goroutine
//   - Failed delivery retried with exponential backoff (1s, 2s, 4s, 8s — max 3 retries)
//   - DeadLetterTopic: after max retries, message published to "<topic>-dlq"
//   - BackpressurePolicy: Drop (non-blocking) or Block (publisher waits)
//   - SubscriptionStats: atomic counters for Delivered, Dropped, Retried, DLQ
//   - Key lesson: async fan-out decouples publisher from slow subscribers
//
// v2 — Ordering keys + push subscriptions + ACL (~300 LoC):
//   - OrderingKey: messages with the same key delivered in publish order per-subscriber
//   - ACL: per-topic allow/deny rules by clientID
//   - SubscriptionType: Pull (channel-based) or Push (broker POSTs to HTTP endpoint)
//   - Key lesson: per-key channels give ordering without global lock on delivery
package pubsub
