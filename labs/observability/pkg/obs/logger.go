package obs

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// LogEntry is the structured JSON line written by Logger.
type LogEntry struct {
	Timestamp string         `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

// Logger writes structured JSON log lines to an io.Writer.
type Logger struct {
	mu     sync.Mutex
	out    io.Writer
	minLvl Level
	fields map[string]any
}

// NewLogger creates a Logger writing to out at minLevel minimum severity.
// If out is nil, os.Stdout is used.
func NewLogger(out io.Writer, minLevel Level) *Logger {
	if out == nil {
		out = os.Stdout
	}
	return &Logger{out: out, minLvl: minLevel, fields: make(map[string]any)}
}

// With returns a new Logger that includes the provided fields in every entry.
// The parent Logger is unchanged.
func (l *Logger) With(fields map[string]any) *Logger {
	l.mu.Lock()
	merged := make(map[string]any, len(l.fields)+len(fields))
	for k, v := range l.fields {
		merged[k] = v
	}
	l.mu.Unlock()
	for k, v := range fields {
		merged[k] = v
	}
	return &Logger{out: l.out, minLvl: l.minLvl, fields: merged}
}

// Debug logs at DEBUG level with context-correlated trace IDs.
func (l *Logger) Debug(ctx context.Context, msg string, fields ...map[string]any) {
	l.log(ctx, LevelDebug, msg, fields...)
}

// Info logs at INFO level.
func (l *Logger) Info(ctx context.Context, msg string, fields ...map[string]any) {
	l.log(ctx, LevelInfo, msg, fields...)
}

// Warn logs at WARN level.
func (l *Logger) Warn(ctx context.Context, msg string, fields ...map[string]any) {
	l.log(ctx, LevelWarn, msg, fields...)
}

// Error logs at ERROR level.
func (l *Logger) Error(ctx context.Context, msg string, fields ...map[string]any) {
	l.log(ctx, LevelError, msg, fields...)
}

func (l *Logger) log(ctx context.Context, lvl Level, msg string, extra ...map[string]any) {
	if lvl < l.minLvl {
		return
	}
	entry := LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     lvl.String(),
		Message:   msg,
	}

	// Correlation: read trace context from the goroutine's context.
	if span := SpanFromContext(ctx); span != nil {
		entry.TraceID = span.TraceID
		entry.SpanID = span.SpanID
	}

	// Merge base fields + call-site fields.
	if len(l.fields) > 0 || len(extra) > 0 {
		merged := make(map[string]any, len(l.fields))
		for k, v := range l.fields {
			merged[k] = v
		}
		for _, m := range extra {
			for k, v := range m {
				merged[k] = v
			}
		}
		if len(merged) > 0 {
			entry.Fields = merged
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')

	l.mu.Lock()
	_, _ = l.out.Write(data)
	l.mu.Unlock()
}
