package stream

import "time"

// Event is a single record entering the stream processor.
// Key partitions events for per-key aggregation.
// Timestamp is the event-time (not processing-time); watermark logic uses this.
type Event struct {
	Key       string
	Value     float64
	Timestamp time.Time
}

// WindowResult is the output of a window aggregation for one key.
type WindowResult struct {
	WindowStart time.Time
	WindowEnd   time.Time
	Key         string
	Count       int
	Sum         float64
	Min         float64
	Max         float64
	Avg         float64
}
