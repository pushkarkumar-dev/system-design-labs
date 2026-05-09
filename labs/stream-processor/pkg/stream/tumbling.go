package stream

import (
	"math"
	"sync"
	"time"
)

// TumblingWindow accumulates events into fixed, non-overlapping time intervals.
// Each window covers exactly [startAt, startAt+size). When the current event
// time crosses startAt+size, the window is flushed and a new one begins.
//
// TumblingWindow is goroutine-safe: multiple producers may call Process
// concurrently while a background goroutine calls Flush periodically.
type TumblingWindow struct {
	mu      sync.Mutex
	size    time.Duration
	startAt time.Time
	buckets map[string][]float64 // key -> values in current window
}

// NewTumblingWindow creates a tumbling window of the given size.
// The first window starts at the time of the first event processed.
func NewTumblingWindow(size time.Duration) *TumblingWindow {
	return &TumblingWindow{
		size:    size,
		buckets: make(map[string][]float64),
	}
}

// Process adds an event to the current window bucket.
// If the event's timestamp lies in a future window, the current window is first
// flushed (results discarded — callers should use Aggregator for result delivery).
// Returns any results emitted by a mid-process flush; nil if no flush occurred.
func (tw *TumblingWindow) Process(e Event) []WindowResult {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	// Initialise the start time on the very first event.
	if tw.startAt.IsZero() {
		tw.startAt = e.Timestamp.Truncate(tw.size)
	}

	// If this event belongs to a later window, flush the current one first.
	var flushed []WindowResult
	for !tw.startAt.IsZero() && e.Timestamp.After(tw.startAt.Add(tw.size).Add(-1)) {
		flushed = append(flushed, tw.flushLocked()...)
		// After flush, advance the window.
		tw.startAt = tw.startAt.Add(tw.size)
		tw.buckets = make(map[string][]float64)
	}

	tw.buckets[e.Key] = append(tw.buckets[e.Key], e.Value)
	return flushed
}

// Flush emits results for all keys in the current window and resets it.
// If no events have been processed yet (startAt is zero), returns nil.
// Safe to call concurrently with Process.
func (tw *TumblingWindow) Flush() []WindowResult {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.startAt.IsZero() {
		return nil
	}
	results := tw.flushLocked()
	tw.startAt = tw.startAt.Add(tw.size)
	tw.buckets = make(map[string][]float64)
	return results
}

// flushLocked computes WindowResult for each key. Must be called with mu held.
func (tw *TumblingWindow) flushLocked() []WindowResult {
	if len(tw.buckets) == 0 {
		return nil
	}
	end := tw.startAt.Add(tw.size)
	results := make([]WindowResult, 0, len(tw.buckets))
	for key, vals := range tw.buckets {
		results = append(results, aggregate(tw.startAt, end, key, vals))
	}
	return results
}

// aggregate computes min/max/sum/avg/count for a slice of float64 values.
func aggregate(start, end time.Time, key string, vals []float64) WindowResult {
	r := WindowResult{
		WindowStart: start,
		WindowEnd:   end,
		Key:         key,
		Count:       len(vals),
		Min:         math.MaxFloat64,
		Max:         -math.MaxFloat64,
	}
	for _, v := range vals {
		r.Sum += v
		if v < r.Min {
			r.Min = v
		}
		if v > r.Max {
			r.Max = v
		}
	}
	if r.Count > 0 {
		r.Avg = r.Sum / float64(r.Count)
	}
	return r
}

// Aggregator runs a tumbling window in a background goroutine.
// Events arrive on Source; results are sent to Sink.
// The window is flushed every windowSize/10 via a ticker.
type Aggregator struct {
	window     *TumblingWindow
	Source     chan Event
	Sink       chan []WindowResult
	stopCh     chan struct{}
	windowSize time.Duration
}

// NewAggregator creates an Aggregator for the given window size.
// Start() must be called to begin processing.
func NewAggregator(windowSize time.Duration) *Aggregator {
	return &Aggregator{
		window:     NewTumblingWindow(windowSize),
		Source:     make(chan Event, 1024),
		Sink:       make(chan []WindowResult, 64),
		stopCh:     make(chan struct{}),
		windowSize: windowSize,
	}
}

// Start launches the background goroutine. Call Stop() to shut down cleanly.
func (a *Aggregator) Start() {
	go a.run()
}

// Stop signals the aggregator to stop and flushes any remaining events.
func (a *Aggregator) Stop() {
	close(a.stopCh)
}

func (a *Aggregator) run() {
	ticker := time.NewTicker(a.windowSize / 10)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopCh:
			// Drain any remaining events from Source before the final flush.
			a.drainAndFlush()
			return
		case e := <-a.Source:
			a.window.Process(e)
		case <-ticker.C:
			if results := a.window.Flush(); len(results) > 0 {
				select {
				case a.Sink <- results:
				default:
					// Drop if sink is full (backpressure not propagated).
				}
			}
		}
	}
}

// drainAndFlush processes all buffered Source events then emits the final flush.
func (a *Aggregator) drainAndFlush() {
	for {
		select {
		case e := <-a.Source:
			a.window.Process(e)
		default:
			if results := a.window.Flush(); len(results) > 0 {
				select {
				case a.Sink <- results:
				default:
				}
			}
			close(a.Sink)
			return
		}
	}
}
