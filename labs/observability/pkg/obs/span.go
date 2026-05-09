package obs

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// SpanStatus indicates whether a span completed successfully or with an error.
type SpanStatus int

const (
	SpanStatusOK    SpanStatus = 0
	SpanStatusError SpanStatus = 1
)

// LogEntry is a timestamped set of key-value fields attached to a Span.
type LogEntry struct {
	Timestamp time.Time
	Fields    map[string]string
}

// Span represents a single unit of work in a distributed trace.
type Span struct {
	TraceID      string
	SpanID       string
	ParentSpanID string // empty string means root span
	Operation    string
	StartTime    time.Time
	Duration     time.Duration
	Tags         map[string]string
	Logs         []LogEntry
	Status       SpanStatus
	finished     bool
	mu           sync.Mutex
	tracer       *Tracer
}

// SetTag adds or updates a string tag on the span.
func (s *Span) SetTag(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

// Log appends a structured log entry to the span.
func (s *Span) Log(fields map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Logs = append(s.Logs, LogEntry{
		Timestamp: time.Now(),
		Fields:    cloneLabels(fields),
	})
}

// SetError marks the span as errored and records the error message.
func (s *Span) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = SpanStatusError
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags["error"] = err.Error()
}

// Finish marks the span as complete and writes it to the SpanStore.
// Safe to call multiple times — only the first call records the span.
func (s *Span) Finish() {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.Duration = time.Since(s.StartTime)
	s.mu.Unlock()

	if s.tracer != nil {
		s.tracer.store.write(s)
	}
}

// ---------------------------------------------------------------------------
// SpanStore — ring buffer of last 10,000 spans
// ---------------------------------------------------------------------------

const spanStoreCapacity = 10_000

// SpanStore is a fixed-size ring buffer that retains the last N finished spans.
// When full, the oldest span is overwritten.
type SpanStore struct {
	mu   sync.Mutex
	buf  [spanStoreCapacity]*Span
	head int // next write position
	size int // current fill level (0..capacity)
}

func (ss *SpanStore) write(s *Span) {
	ss.mu.Lock()
	ss.buf[ss.head] = s
	ss.head = (ss.head + 1) % spanStoreCapacity
	if ss.size < spanStoreCapacity {
		ss.size++
	}
	ss.mu.Unlock()
}

// All returns a copy of all stored spans in insertion order (oldest first).
func (ss *SpanStore) All() []*Span {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	out := make([]*Span, 0, ss.size)
	if ss.size < spanStoreCapacity {
		// Buffer not yet full: 0..head-1 are valid.
		for i := 0; i < ss.size; i++ {
			out = append(out, ss.buf[i])
		}
	} else {
		// Buffer full: head points to oldest.
		for i := 0; i < spanStoreCapacity; i++ {
			idx := (ss.head + i) % spanStoreCapacity
			out = append(out, ss.buf[idx])
		}
	}
	return out
}

// FindByTraceID returns all spans belonging to a trace.
func (ss *SpanStore) FindByTraceID(traceID string) []*Span {
	all := ss.All()
	out := make([]*Span, 0)
	for _, s := range all {
		if s.TraceID == traceID {
			out = append(out, s)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tracer
// ---------------------------------------------------------------------------

// Tracer creates and manages spans.
type Tracer struct {
	store *SpanStore
}

// NewTracer creates a Tracer backed by a fresh SpanStore.
func NewTracer() *Tracer {
	return &Tracer{store: &SpanStore{}}
}

// Store returns the underlying SpanStore for inspection.
func (t *Tracer) Store() *SpanStore { return t.store }

// StartSpan creates a new Span. If parent is non-nil the span is a child;
// otherwise it is a root span with a new trace ID.
func (t *Tracer) StartSpan(operation string, parent *Span) *Span {
	s := &Span{
		SpanID:    newID(8),
		Operation: operation,
		StartTime: time.Now(),
		Tags:      make(map[string]string),
		tracer:    t,
	}
	if parent != nil {
		s.TraceID = parent.TraceID
		s.ParentSpanID = parent.SpanID
	} else {
		s.TraceID = newID(16)
	}
	return s
}

// StartSpanFromContext creates a span whose parent is encoded in ctx
// (as set by middleware). Falls back to a root span if no parent exists.
func (t *Tracer) StartSpanFromContext(ctx interface{ Value(any) any }, operation string) *Span {
	if parent, ok := ctx.Value(contextKeySpan{}).(*Span); ok && parent != nil {
		return t.StartSpan(operation, parent)
	}
	return t.StartSpan(operation, nil)
}

// newID generates a random hex string of byteLen bytes (2*byteLen hex chars).
func newID(byteLen int) string {
	b := make([]byte, byteLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// contextKeySpan is the key used to store *Span in a context.Value chain.
type contextKeySpan struct{}
