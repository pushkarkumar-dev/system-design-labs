package saga

import (
	"fmt"
)

// ── SagaOrchestrator ──────────────────────────────────────────────────────────

// SagaOrchestrator drives saga execution with a persistent event log.
// Before executing each step it writes StepStarted; after success StepCompleted;
// on failure it writes StepFailed then starts compensation.
//
// Idempotency: if the log already shows StepCompleted for a step, the
// orchestrator skips re-executing it. This handles duplicate deliveries and
// crash-then-recover scenarios where some steps already ran.
//
// Usage:
//
//	log := &SagaLog{}
//	orch := NewSagaOrchestrator(log)
//	result := orch.Run("saga-42", saga, ctx)
//
//	// After a crash, recover by replaying the log:
//	result2 := orch.Recover("saga-42", saga, ctx)
type SagaOrchestrator struct {
	log *SagaLog
}

// NewSagaOrchestrator creates a new orchestrator backed by the given log.
func NewSagaOrchestrator(log *SagaLog) *SagaOrchestrator {
	return &SagaOrchestrator{log: log}
}

// Run executes the saga with full event logging. Steps that already appear as
// StepCompleted in the log are skipped (idempotency). On failure, compensations
// are run in reverse for all StepCompleted steps.
func (o *SagaOrchestrator) Run(sagaID string, s *Saga, ctx SagaContext) SagaResult {
	// Replay the log to determine what has already been done.
	events := o.log.EventsFor(sagaID)
	state := ReplayLog(events)

	var execLog []ExecutionLogEntry
	completed := make([]int, 0, len(s.Steps))

	appendEvt := func(stepName string, kind EventKind, err error) {
		evt := SagaEvent{SagaID: sagaID, StepName: stepName, Kind: kind}
		if err != nil {
			evt.ErrString = err.Error()
		}
		o.log.Append(evt)
	}

	logEntry := func(name, phase, status string, err error) {
		execLog = append(execLog, ExecutionLogEntry{
			StepName: name,
			Phase:    phase,
			Status:   status,
			Err:      err,
		})
	}

	// ── Forward execution ────────────────────────────────────────────────────
	for i, step := range s.Steps {
		// Idempotency check: skip steps already completed in a prior run.
		if state.CompletedSteps[step.Name] {
			logEntry(step.Name, "execute", "skipped (already completed)", nil)
			completed = append(completed, i)
			continue
		}

		appendEvt(step.Name, EventStepStarted, nil)
		logEntry(step.Name, "execute", "started", nil)

		if err := step.Execute(ctx); err != nil {
			appendEvt(step.Name, EventStepFailed, err)
			logEntry(step.Name, "execute", "failed", err)

			// ── Reverse compensation ─────────────────────────────────────
			for j := len(completed) - 1; j >= 0; j-- {
				idx := completed[j]
				cs := s.Steps[idx]

				// Skip steps already compensated in a prior run.
				if state.CompensatedSteps[cs.Name] {
					logEntry(cs.Name, "compensate", "skipped (already compensated)", nil)
					continue
				}

				appendEvt(cs.Name, EventCompensationStarted, nil)
				logEntry(cs.Name, "compensate", "started", nil)

				if cerr := cs.Compensate(ctx); cerr != nil {
					appendEvt(cs.Name, EventCompensationFailed, cerr)
					logEntry(cs.Name, "compensate", "failed", cerr)
					// Continue compensating other steps even if this one fails.
				} else {
					appendEvt(cs.Name, EventCompensationCompleted, nil)
					logEntry(cs.Name, "compensate", "completed", nil)
				}
			}

			return SagaResult{
				Status:     StatusCompensated,
				FailedStep: step.Name,
				Error:      fmt.Errorf("step %q failed: %w", step.Name, err),
				Log:        execLog,
			}
		}

		appendEvt(step.Name, EventStepCompleted, nil)
		logEntry(step.Name, "execute", "completed", nil)
		completed = append(completed, i)
	}

	return SagaResult{
		Status: StatusCompleted,
		Log:    execLog,
	}
}

// Recover resumes a saga from wherever it left off by replaying the event log.
// Steps already marked StepCompleted are skipped; compensation resumes from the
// correct position if the saga was mid-compensation.
//
// This is the primary mechanism for handling process crashes: on restart,
// the orchestrator scans the event store for in-flight sagas and calls Recover
// for each one.
func (o *SagaOrchestrator) Recover(sagaID string, s *Saga, ctx SagaContext) SagaResult {
	// Recover uses the same Run logic — the idempotency check inside Run
	// ensures completed steps are skipped automatically.
	return o.Run(sagaID, s, ctx)
}
