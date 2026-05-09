package saga

import (
	"fmt"
	"time"
)

// ── SagaContext ───────────────────────────────────────────────────────────────

// SagaContext is a typed key-value store passed through all saga steps.
// Steps use it to share state (e.g., orderId produced by step 1 consumed by step 2).
// It is not goroutine-safe; the orchestrator drives steps sequentially.
type SagaContext map[string]any

// Set stores a value under key.
func (c SagaContext) Set(key string, val any) {
	c[key] = val
}

// Get retrieves a value by key. Returns nil if absent.
func (c SagaContext) Get(key string) any {
	return c[key]
}

// GetString retrieves a string value. Returns "" if absent or wrong type.
func (c SagaContext) GetString(key string) string {
	v, _ := c[key].(string)
	return v
}

// ── Step ─────────────────────────────────────────────────────────────────────

// Step is one unit of work in a saga. Both Execute and Compensate receive the
// shared SagaContext so they can read the artifacts produced by prior steps.
//
// Compensate must be idempotent — the orchestrator may call it more than once.
// Compensate should not return an error that stops other compensations; the
// orchestrator logs compensation failures but always continues compensating.
type Step struct {
	// Name is a human-readable label used in logs and results.
	Name string

	// Execute performs the forward action. Returns an error on failure.
	// Return a RetryableError (v2) to trigger retry with backoff.
	// Return any other error to trigger immediate compensation.
	Execute func(ctx SagaContext) error

	// Compensate undoes the action performed by Execute. Called in reverse
	// order when any later step fails.
	Compensate func(ctx SagaContext) error
}

// ── ExecutionLogEntry ─────────────────────────────────────────────────────────

// ExecutionLogEntry records what happened at each step during a saga run.
type ExecutionLogEntry struct {
	StepName  string
	Phase     string // "execute" or "compensate"
	Status    string // "started", "completed", "failed"
	Err       error
	Timestamp time.Time
}

func (e ExecutionLogEntry) String() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s %s FAILED: %v", e.Timestamp.Format(time.RFC3339), e.StepName, e.Phase, e.Err)
	}
	return fmt.Sprintf("[%s] %s %s %s", e.Timestamp.Format(time.RFC3339), e.StepName, e.Phase, e.Status)
}

// ── SagaStatus ───────────────────────────────────────────────────────────────

// SagaStatus represents the terminal state of a saga run.
type SagaStatus string

const (
	// StatusCompleted means all steps executed successfully.
	StatusCompleted SagaStatus = "completed"
	// StatusFailed means a step failed and compensation is still running or did not exist.
	StatusFailed SagaStatus = "failed"
	// StatusCompensated means a step failed and all compensations completed.
	StatusCompensated SagaStatus = "compensated"
)

// ── SagaResult ───────────────────────────────────────────────────────────────

// SagaResult is returned by Saga.Run. It captures the terminal status, which
// step failed (if any), the original error, and the full execution log.
type SagaResult struct {
	Status      SagaStatus
	FailedStep  string
	Error       error
	Log         []ExecutionLogEntry
}

// ── Saga ─────────────────────────────────────────────────────────────────────

// Saga is an ordered list of steps. Run executes them in order; on first
// failure it runs compensations in reverse, then returns a SagaResult.
//
// Usage:
//
//	s := &Saga{
//	    Steps: []Step{
//	        {Name: "InventoryReserve", Execute: reserveInventory, Compensate: releaseInventory},
//	        {Name: "PaymentCharge",    Execute: chargePayment,    Compensate: refundPayment},
//	        {Name: "ShipmentCreate",   Execute: createShipment,   Compensate: cancelShipment},
//	    },
//	}
//	ctx := SagaContext{"orderId": "ord-42"}
//	result := s.Run(ctx)
type Saga struct {
	Steps []Step
}

// Run executes the saga forward. If any step's Execute returns an error,
// Run compensates all previously completed steps in reverse order.
// Compensation failures are logged but do not abort further compensations.
func (s *Saga) Run(ctx SagaContext) SagaResult {
	var log []ExecutionLogEntry
	completed := make([]int, 0, len(s.Steps))

	logEntry := func(name, phase, status string, err error) {
		log = append(log, ExecutionLogEntry{
			StepName:  name,
			Phase:     phase,
			Status:    status,
			Err:       err,
			Timestamp: time.Now(),
		})
	}

	// ── Forward execution ────────────────────────────────────────────────────
	for i, step := range s.Steps {
		logEntry(step.Name, "execute", "started", nil)

		if err := step.Execute(ctx); err != nil {
			logEntry(step.Name, "execute", "failed", err)

			// ── Reverse compensation ─────────────────────────────────────
			for j := len(completed) - 1; j >= 0; j-- {
				idx := completed[j]
				cs := s.Steps[idx]
				logEntry(cs.Name, "compensate", "started", nil)

				if cerr := cs.Compensate(ctx); cerr != nil {
					logEntry(cs.Name, "compensate", "failed", cerr)
					// Log but continue — must attempt all compensations.
				} else {
					logEntry(cs.Name, "compensate", "completed", nil)
				}
			}

			return SagaResult{
				Status:     StatusCompensated,
				FailedStep: step.Name,
				Error:      fmt.Errorf("step %q failed: %w", step.Name, err),
				Log:        log,
			}
		}

		logEntry(step.Name, "execute", "completed", nil)
		completed = append(completed, i)
	}

	return SagaResult{
		Status: StatusCompleted,
		Log:    log,
	}
}
