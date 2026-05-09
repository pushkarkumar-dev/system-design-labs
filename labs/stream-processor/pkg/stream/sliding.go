package stream

import (
	"sort"
	"sync"
	"time"
)

// SlidingWindow implements overlapping time windows that fire every step duration
// and each cover the last size duration of event time.
//
// Example: size=5m, step=1m — a window fires every minute, covering the previous 5 minutes.
// At event time T, all windows [T-4m, T+1m), [T-3m, T+2m), ... [T, T+5m) are active.
//
// Events are buffered in a sorted slice ordered by Timestamp. When the watermark
// advances past a window's end time, that window is flushed and its results emitted.
type SlidingWindow struct {
	mu              sync.Mutex
	size            time.Duration
	step            time.Duration
	allowedLateness time.Duration

	// sortedBuf holds all events not yet flushed, sorted by Timestamp.
	sortedBuf []Event

	// flushedUpTo is the latest window-end we have already emitted.
	flushedUpTo time.Time

	watermark *Watermark
}

// NewSlidingWindow creates a sliding window aggregator.
func NewSlidingWindow(size, step, allowedLateness time.Duration) *SlidingWindow {
	return &SlidingWindow{
		size:            size,
		step:            step,
		allowedLateness: allowedLateness,
		watermark:       NewWatermark(allowedLateness),
	}
}

// insertSorted inserts e into sw.sortedBuf maintaining ascending Timestamp order.
func (sw *SlidingWindow) insertSorted(e Event) {
	idx := sort.Search(len(sw.sortedBuf), func(i int) bool {
		return !sw.sortedBuf[i].Timestamp.Before(e.Timestamp)
	})
	sw.sortedBuf = append(sw.sortedBuf, Event{})
	copy(sw.sortedBuf[idx+1:], sw.sortedBuf[idx:])
	sw.sortedBuf[idx] = e
}

// StreamProcessor wires a SlidingWindow with a source channel, a result sink,
// and a late-event side output channel.
type StreamProcessor struct {
	cfg    ProcessorConfig
	window *SlidingWindow
	Source  chan Event
	Sink    chan WindowResult
	LateSink chan Event
	stopCh  chan struct{}
}

// ProcessorConfig configures a StreamProcessor.
type ProcessorConfig struct {
	WindowSize      time.Duration
	WindowStep      time.Duration
	AllowedLateness time.Duration
}

// NewStreamProcessor creates a StreamProcessor with the given config.
func NewStreamProcessor(cfg ProcessorConfig) *StreamProcessor {
	return &StreamProcessor{
		cfg:      cfg,
		window:   NewSlidingWindow(cfg.WindowSize, cfg.WindowStep, cfg.AllowedLateness),
		Source:   make(chan Event, 4096),
		Sink:     make(chan WindowResult, 4096),
		LateSink: make(chan Event, 1024),
		stopCh:   make(chan struct{}),
	}
}

// Start launches background processing goroutines.
func (sp *StreamProcessor) Start() {
	go sp.run()
}

// Stop signals the processor to finish.
func (sp *StreamProcessor) Stop() {
	close(sp.stopCh)
}

func (sp *StreamProcessor) run() {
	ticker := time.NewTicker(sp.cfg.WindowStep / 2)
	defer ticker.Stop()
	defer close(sp.Sink)

	for {
		select {
		case <-sp.stopCh:
			sp.flushAll()
			return
		case e, ok := <-sp.Source:
			if !ok {
				sp.flushAll()
				return
			}
			sp.processEvent(e)
		case <-ticker.C:
			sp.flushMature()
		}
	}
}

func (sp *StreamProcessor) processEvent(e Event) {
	w := sp.window
	w.mu.Lock()
	defer w.mu.Unlock()

	wm, _ := w.watermark.Advance(e.Timestamp)

	// Route too-late events (before watermark) to side output.
	if !wm.IsZero() && e.Timestamp.Before(wm) {
		select {
		case sp.LateSink <- e:
		default:
		}
		return
	}

	w.insertSorted(e)
}

// flushMature emits results for all windows whose end time has been passed by the watermark.
func (sp *StreamProcessor) flushMature() {
	w := sp.window
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.sortedBuf) == 0 {
		return
	}

	currentWM := w.watermark.Current()
	if currentWM.IsZero() {
		return
	}

	// Determine the earliest event time we have.
	minEventTime := w.sortedBuf[0].Timestamp

	// Enumerate all window slots that have ended before the watermark.
	// A sliding window [start, start+size) fires when watermark >= start+size,
	// i.e. start+size <= watermark.
	// Windows are aligned to step boundaries.
	// Go back by (size - step) so that windows starting before minEventTime
	// but still containing events at minEventTime are included.
	firstWindowStart := minEventTime.Truncate(w.step).Add(-(w.size - w.step))

	for windowStart := firstWindowStart; windowStart.Add(w.size).Before(currentWM) || windowStart.Add(w.size).Equal(currentWM); windowStart = windowStart.Add(w.step) {
		windowEnd := windowStart.Add(w.size)

		// Skip already-flushed windows.
		if !windowEnd.After(w.flushedUpTo) {
			continue
		}

		// Collect events that fall inside [windowStart, windowEnd).
		buckets := make(map[string][]float64)
		for _, e := range w.sortedBuf {
			if (e.Timestamp.Equal(windowStart) || e.Timestamp.After(windowStart)) &&
				e.Timestamp.Before(windowEnd) {
				buckets[e.Key] = append(buckets[e.Key], e.Value)
			}
		}

		for key, vals := range buckets {
			result := aggregate(windowStart, windowEnd, key, vals)
			select {
			case sp.Sink <- result:
			default:
			}
		}

		w.flushedUpTo = windowEnd
	}

	// Evict events that are no longer needed by any future window.
	// The earliest window that can still receive events starts at: watermark - size + step.
	cutoff := currentWM.Add(-w.size)
	keep := 0
	for _, e := range w.sortedBuf {
		if e.Timestamp.After(cutoff) || e.Timestamp.Equal(cutoff) {
			w.sortedBuf[keep] = e
			keep++
		}
	}
	w.sortedBuf = w.sortedBuf[:keep]
}

// flushAll emits all remaining buffered events into a final window.
func (sp *StreamProcessor) flushAll() {
	w := sp.window
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.sortedBuf) == 0 {
		return
	}

	minTime := w.sortedBuf[0].Timestamp
	maxTime := w.sortedBuf[len(w.sortedBuf)-1].Timestamp
	windowStart := minTime.Truncate(w.step)
	windowEnd := maxTime.Add(w.step).Truncate(w.step)

	buckets := make(map[string][]float64)
	for _, e := range w.sortedBuf {
		buckets[e.Key] = append(buckets[e.Key], e.Value)
	}
	for key, vals := range buckets {
		result := aggregate(windowStart, windowEnd, key, vals)
		select {
		case sp.Sink <- result:
		default:
		}
	}
	w.sortedBuf = nil
}
