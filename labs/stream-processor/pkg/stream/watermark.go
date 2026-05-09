package stream

import (
	"sync"
	"sync/atomic"
	"time"
)

// Watermark tracks the maximum event time seen and derives the current watermark
// as maxEventTimeSeen - allowedLateness.
//
// Events whose timestamps are strictly before the watermark are "too late" and
// should be routed to a side output rather than processed normally.
type Watermark struct {
	mu              sync.Mutex
	maxEventTime    time.Time
	allowedLateness time.Duration

	// watermarkNanos stores watermark as Unix nanoseconds for atomic reads.
	watermarkNanos atomic.Int64
}

// NewWatermark creates a watermark tracker with the given allowed lateness.
func NewWatermark(allowedLateness time.Duration) *Watermark {
	return &Watermark{
		allowedLateness: allowedLateness,
	}
}

// Advance updates the watermark with a new event timestamp.
// Returns the new watermark time and whether it advanced.
func (w *Watermark) Advance(eventTime time.Time) (time.Time, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if eventTime.After(w.maxEventTime) {
		w.maxEventTime = eventTime
		newWatermark := w.maxEventTime.Add(-w.allowedLateness)
		w.watermarkNanos.Store(newWatermark.UnixNano())
		return newWatermark, true
	}
	return w.current(), false
}

// Current returns the current watermark time (lock-free read).
func (w *Watermark) Current() time.Time {
	return w.current()
}

func (w *Watermark) current() time.Time {
	ns := w.watermarkNanos.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// IsLate returns true if the event timestamp is strictly before the watermark.
func (w *Watermark) IsLate(eventTime time.Time) bool {
	wm := w.current()
	return !wm.IsZero() && eventTime.Before(wm)
}
