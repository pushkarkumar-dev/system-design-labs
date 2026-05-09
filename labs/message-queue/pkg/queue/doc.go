// Package queue implements a simplified SQS-like message queue in three
// progressive versions.
//
// v0 — In-memory FIFO queue with visibility timeout.
//
//	Key lesson: ReceiptHandle-based deletion enables at-least-once delivery.
//	A message is "invisible" while in-flight; it reappears if the consumer
//	crashes before calling DeleteMessage.
//
// v1 — Dead-letter queue and delayed delivery.
//
//	Key lesson: after maxReceiveCount receives without deletion, messages are
//	automatically moved to a DLQ instead of looping forever. Delayed delivery
//	lets producers schedule messages for future visibility.
//
// v2 — FIFO queue with deduplication and long polling.
//
//	Key lesson: sync.Cond eliminates busy-wait on empty queues; per-group
//	message ordering avoids the global lock bottleneck of a pure FIFO queue.
package queue
