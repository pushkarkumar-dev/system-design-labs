package saga

import (
	"fmt"
	"sync"
	"time"
)

// ── Event types ───────────────────────────────────────────────────────────────

// EventKind identifies the type of a SagaEvent.
type EventKind string

const (
	EventStepStarted           EventKind = "StepStarted"
	EventStepCompleted         EventKind = "StepCompleted"
	EventStepFailed            EventKind = "StepFailed"
	EventCompensationStarted   EventKind = "CompensationStarted"
	EventCompensationCompleted EventKind = "CompensationCompleted"
	EventCompensationFailed    EventKind = "CompensationFailed"
)

// SagaEvent is a single entry in the saga event log.
// Every mutation to saga state — step execution, completion, compensation —
// is recorded as an immutable event. Recovery replays these events to
// reconstruct the saga state without re-executing completed steps.
type SagaEvent struct {
	SagaID    string
	StepName  string
	Kind      EventKind
	ErrString string    // non-empty for failure events
	At        time.Time // wall-clock timestamp
}

func (e SagaEvent) String() string {
	ts := e.At.Format("15:04:05.000")
	if e.ErrString != "" {
		return fmt.Sprintf("[%s] %s/%s %s err=%s", ts, e.SagaID, e.StepName, e.Kind, e.ErrString)
	}
	return fmt.Sprintf("[%s] %s/%s %s", ts, e.SagaID, e.StepName, e.Kind)
}

// ── SagaLog ───────────────────────────────────────────────────────────────────

// SagaLog is an append-only, in-memory event log that simulates durable storage.
// In production this would be a PostgreSQL table or an event broker topic.
//
// Concurrent writes are safe; reads via Events() return a snapshot copy.
type SagaLog struct {
	mu     sync.Mutex
	events []SagaEvent
}

// Append adds an event to the log. This is the only mutation operation.
func (l *SagaLog) Append(evt SagaEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	evt.At = time.Now()
	l.events = append(l.events, evt)
}

// Events returns a snapshot copy of all events. Safe to call concurrently.
func (l *SagaLog) Events() []SagaEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	snap := make([]SagaEvent, len(l.events))
	copy(snap, l.events)
	return snap
}

// EventsFor returns all events associated with a specific sagaID.
func (l *SagaLog) EventsFor(sagaID string) []SagaEvent {
	all := l.Events()
	var result []SagaEvent
	for _, e := range all {
		if e.SagaID == sagaID {
			result = append(result, e)
		}
	}
	return result
}

// ── SagaState reconstructed from log ─────────────────────────────────────────

// SagaRecoveryState is the result of replaying a saga's event log.
// The orchestrator uses it to know which steps to skip and whether
// to resume forward execution or continue compensating.
type SagaRecoveryState struct {
	// CompletedSteps lists step names that have a StepCompleted event.
	// The orchestrator will skip executing these on resume.
	CompletedSteps map[string]bool

	// FailedStep is the step name that has a StepFailed event, if any.
	FailedStep string

	// CompensatedSteps lists step names that have a CompensationCompleted event.
	CompensatedSteps map[string]bool

	// IsComplete is true when all steps have a StepCompleted event with no failures.
	IsComplete bool
}

// ReplayLog reconstructs SagaRecoveryState from the events for a given sagaID.
func ReplayLog(events []SagaEvent) SagaRecoveryState {
	state := SagaRecoveryState{
		CompletedSteps:   make(map[string]bool),
		CompensatedSteps: make(map[string]bool),
	}

	for _, e := range events {
		switch e.Kind {
		case EventStepCompleted:
			state.CompletedSteps[e.StepName] = true
		case EventStepFailed:
			state.FailedStep = e.StepName
		case EventCompensationCompleted:
			state.CompensatedSteps[e.StepName] = true
		}
	}

	return state
}
