// Package stream implements a stream processor in three stages:
//
//   - v0: Tumbling window aggregation — fixed non-overlapping time windows per key.
//   - v1: Sliding windows with watermarks and out-of-order event handling.
//   - v2: Exactly-once delivery via two-phase commit with atomic checkpoints.
//
// Usage:
//
//	// v0 — tumbling window
//	tw := stream.NewTumblingWindow(time.Minute)
//	tw.Process(stream.Event{Key: "sensor-1", Value: 42.0, Timestamp: time.Now()})
//	results := tw.Flush()
//
//	// v1 — sliding window with watermark
//	sp := stream.NewStreamProcessor(stream.ProcessorConfig{
//	    WindowSize:     5 * time.Minute,
//	    WindowStep:     1 * time.Minute,
//	    AllowedLateness: 5 * time.Second,
//	})
//	sp.Start()
//
//	// v2 — exactly-once via 2PC
//	coord := stream.NewTxnCoordinator("/tmp/checkpoint.json")
//	coord.ProcessBatch(batch, sink)
package stream
