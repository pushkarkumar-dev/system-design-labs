package saga

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func okStep(name string) Step {
	return Step{
		Name:       name,
		Execute:    func(ctx SagaContext) error { return nil },
		Compensate: func(ctx SagaContext) error { return nil },
	}
}

func failStep(name string, err error) Step {
	return Step{
		Name:       name,
		Execute:    func(ctx SagaContext) error { return err },
		Compensate: func(ctx SagaContext) error { return nil },
	}
}

func trackingStep(name string, executed, compensated *[]string) Step {
	return Step{
		Name: name,
		Execute: func(ctx SagaContext) error {
			*executed = append(*executed, name)
			return nil
		},
		Compensate: func(ctx SagaContext) error {
			*compensated = append(*compensated, name)
			return nil
		},
	}
}

// ── v0: In-memory Saga ────────────────────────────────────────────────────────

// Test 1: Happy path — all steps complete, result is StatusCompleted.
func TestSaga_HappyPath(t *testing.T) {
	s := &Saga{
		Steps: []Step{
			okStep("InventoryReserve"),
			okStep("PaymentCharge"),
			okStep("ShipmentCreate"),
		},
	}
	ctx := SagaContext{}
	result := s.Run(ctx)

	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", result.Status)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}
	if result.FailedStep != "" {
		t.Errorf("expected no failed step, got %s", result.FailedStep)
	}
}

// Test 2: Failure at step 2 — only step 1's compensation runs.
func TestSaga_FailureAtStep2_OneCompensation(t *testing.T) {
	var executed, compensated []string

	s := &Saga{
		Steps: []Step{
			trackingStep("InventoryReserve", &executed, &compensated),
			{
				Name: "PaymentCharge",
				Execute: func(ctx SagaContext) error {
					executed = append(executed, "PaymentCharge")
					return errors.New("card declined")
				},
				Compensate: func(ctx SagaContext) error {
					compensated = append(compensated, "PaymentCharge")
					return nil
				},
			},
			trackingStep("ShipmentCreate", &executed, &compensated),
		},
	}
	ctx := SagaContext{}
	result := s.Run(ctx)

	if result.Status != StatusCompensated {
		t.Errorf("expected StatusCompensated, got %s", result.Status)
	}
	if result.FailedStep != "PaymentCharge" {
		t.Errorf("expected FailedStep=PaymentCharge, got %s", result.FailedStep)
	}

	// Only step 1 executed and completed before failure.
	if len(executed) != 2 { // InventoryReserve + PaymentCharge (failed)
		t.Errorf("expected 2 executed, got %d: %v", len(executed), executed)
	}
	// Only step 1 needs compensation (step 2 failed, step 3 never ran).
	if len(compensated) != 1 || compensated[0] != "InventoryReserve" {
		t.Errorf("expected [InventoryReserve] compensated, got %v", compensated)
	}
}

// Test 3: Failure at step 3 — steps 1 and 2 compensated in reverse.
func TestSaga_FailureAtStep3_TwoCompensations(t *testing.T) {
	var compensated []string

	s := &Saga{
		Steps: []Step{
			{
				Name:       "InventoryReserve",
				Execute:    func(ctx SagaContext) error { return nil },
				Compensate: func(ctx SagaContext) error { compensated = append(compensated, "InventoryRelease"); return nil },
			},
			{
				Name:       "PaymentCharge",
				Execute:    func(ctx SagaContext) error { return nil },
				Compensate: func(ctx SagaContext) error { compensated = append(compensated, "PaymentRefund"); return nil },
			},
			{
				Name:    "ShipmentCreate",
				Execute: func(ctx SagaContext) error { return errors.New("warehouse offline") },
				Compensate: func(ctx SagaContext) error {
					compensated = append(compensated, "ShipmentCancel")
					return nil
				},
			},
		},
	}
	ctx := SagaContext{}
	result := s.Run(ctx)

	if result.Status != StatusCompensated {
		t.Errorf("expected StatusCompensated, got %s", result.Status)
	}

	// Compensations run in reverse: PaymentRefund then InventoryRelease.
	if len(compensated) != 2 {
		t.Fatalf("expected 2 compensations, got %d: %v", len(compensated), compensated)
	}
	if compensated[0] != "PaymentRefund" {
		t.Errorf("first compensation should be PaymentRefund, got %s", compensated[0])
	}
	if compensated[1] != "InventoryRelease" {
		t.Errorf("second compensation should be InventoryRelease, got %s", compensated[1])
	}
}

// Test 4: Compensation failure is logged but does not re-compensate.
func TestSaga_CompensationFailureIsLogged(t *testing.T) {
	s := &Saga{
		Steps: []Step{
			{
				Name:    "InventoryReserve",
				Execute: func(ctx SagaContext) error { return nil },
				Compensate: func(ctx SagaContext) error {
					return errors.New("inventory service down")
				},
			},
			{
				Name:    "PaymentCharge",
				Execute: func(ctx SagaContext) error { return errors.New("card declined") },
				Compensate: func(ctx SagaContext) error {
					t.Error("PaymentCharge compensation should not be called — step failed")
					return nil
				},
			},
		},
	}
	ctx := SagaContext{}
	result := s.Run(ctx)

	if result.Status != StatusCompensated {
		t.Errorf("expected StatusCompensated even when compensation fails, got %s", result.Status)
	}

	// Log should contain the compensation failure.
	var foundCompFail bool
	for _, entry := range result.Log {
		if entry.StepName == "InventoryReserve" && entry.Phase == "compensate" && entry.Status == "failed" {
			foundCompFail = true
		}
	}
	if !foundCompFail {
		t.Error("expected compensation failure in log for InventoryReserve")
	}
}

// Test 5: Context state flows between steps.
func TestSaga_ContextFlowsBetwenSteps(t *testing.T) {
	s := &Saga{
		Steps: []Step{
			{
				Name: "InventoryReserve",
				Execute: func(ctx SagaContext) error {
					ctx.Set("reservationRef", "res-001")
					return nil
				},
				Compensate: func(ctx SagaContext) error { return nil },
			},
			{
				Name: "PaymentCharge",
				Execute: func(ctx SagaContext) error {
					ref := ctx.GetString("reservationRef")
					if ref == "" {
						return errors.New("missing reservationRef from context")
					}
					ctx.Set("paymentRef", "pay-"+ref)
					return nil
				},
				Compensate: func(ctx SagaContext) error { return nil },
			},
			{
				Name: "ShipmentCreate",
				Execute: func(ctx SagaContext) error {
					payRef := ctx.GetString("paymentRef")
					if payRef == "" {
						return errors.New("missing paymentRef from context")
					}
					ctx.Set("shipmentID", "ship-"+payRef)
					return nil
				},
				Compensate: func(ctx SagaContext) error { return nil },
			},
		},
	}
	ctx := SagaContext{}
	result := s.Run(ctx)

	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s (err=%v)", result.Status, result.Error)
	}
	if ctx.GetString("shipmentID") != "ship-pay-res-001" {
		t.Errorf("unexpected shipmentID: %s", ctx.GetString("shipmentID"))
	}
}

// Test 6: Empty saga completes with no steps.
func TestSaga_EmptySaga(t *testing.T) {
	s := &Saga{}
	result := s.Run(SagaContext{})
	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted for empty saga, got %s", result.Status)
	}
}

// Test 7: Single-step saga failure leaves nothing to compensate.
func TestSaga_SingleStepFailure(t *testing.T) {
	compensated := false
	s := &Saga{
		Steps: []Step{
			{
				Name:       "OnlyStep",
				Execute:    func(ctx SagaContext) error { return errors.New("always fails") },
				Compensate: func(ctx SagaContext) error { compensated = true; return nil },
			},
		},
	}
	result := s.Run(SagaContext{})
	if result.Status != StatusCompensated {
		t.Errorf("expected StatusCompensated, got %s", result.Status)
	}
	// The failed step itself is not compensated.
	if compensated {
		t.Error("failed step should not be compensated")
	}
}

// Test 8: Log entries record the correct sequence.
func TestSaga_LogEntrySequence(t *testing.T) {
	s := &Saga{
		Steps: []Step{
			okStep("Step1"),
			failStep("Step2", errors.New("boom")),
		},
	}
	result := s.Run(SagaContext{})

	// Expected: Step1 started, Step1 completed, Step2 started, Step2 failed,
	// Step1 compensate started, Step1 compensate completed.
	if len(result.Log) != 6 {
		t.Errorf("expected 6 log entries, got %d:", len(result.Log))
		for i, e := range result.Log {
			t.Logf("  [%d] %s", i, e)
		}
	}
}

// ── v1: SagaOrchestrator + event log ─────────────────────────────────────────

// Test 9: Recovery skips completed steps.
func TestOrchestrator_RecoverySkipsCompletedSteps(t *testing.T) {
	log := &SagaLog{}
	orch := NewSagaOrchestrator(log)

	var step1Calls int

	s := &Saga{
		Steps: []Step{
			{
				Name: "Step1",
				Execute: func(ctx SagaContext) error {
					step1Calls++
					return nil
				},
				Compensate: func(ctx SagaContext) error { return nil },
			},
			okStep("Step2"),
		},
	}

	// First run — both steps execute.
	result := orch.Run("saga-1", s, SagaContext{})
	if result.Status != StatusCompleted {
		t.Fatalf("first run failed: %v", result.Error)
	}
	if step1Calls != 1 {
		t.Errorf("Step1 should have run once, ran %d times", step1Calls)
	}

	// Second run (simulates recovery) — Step1 should be skipped.
	result2 := orch.Recover("saga-1", s, SagaContext{})
	if result2.Status != StatusCompleted {
		t.Fatalf("recovery failed: %v", result2.Error)
	}
	if step1Calls != 1 {
		t.Errorf("Step1 should still be 1 after recovery, got %d", step1Calls)
	}
}

// Test 10: Recovery resumes mid-saga.
func TestOrchestrator_RecoveryResumesMidSaga(t *testing.T) {
	log := &SagaLog{}
	orch := NewSagaOrchestrator(log)

	// Manually inject a StepCompleted event for Step1 to simulate a prior partial run.
	log.Append(SagaEvent{SagaID: "saga-2", StepName: "Step1", Kind: EventStepCompleted})

	var step1Calls, step2Calls int
	s := &Saga{
		Steps: []Step{
			{
				Name:       "Step1",
				Execute:    func(ctx SagaContext) error { step1Calls++; return nil },
				Compensate: func(ctx SagaContext) error { return nil },
			},
			{
				Name:       "Step2",
				Execute:    func(ctx SagaContext) error { step2Calls++; return nil },
				Compensate: func(ctx SagaContext) error { return nil },
			},
		},
	}

	result := orch.Recover("saga-2", s, SagaContext{})
	if result.Status != StatusCompleted {
		t.Fatalf("recovery failed: %v", result.Error)
	}
	if step1Calls != 0 {
		t.Errorf("Step1 should be skipped on recovery, ran %d times", step1Calls)
	}
	if step2Calls != 1 {
		t.Errorf("Step2 should run once on recovery, ran %d times", step2Calls)
	}
}

// Test 11: Duplicate step execution is skipped.
func TestOrchestrator_DuplicateStepSkipped(t *testing.T) {
	log := &SagaLog{}
	orch := NewSagaOrchestrator(log)

	var calls int
	s := &Saga{
		Steps: []Step{
			{
				Name:       "IdempotentStep",
				Execute:    func(ctx SagaContext) error { calls++; return nil },
				Compensate: func(ctx SagaContext) error { return nil },
			},
		},
	}

	orch.Run("saga-3", s, SagaContext{})
	orch.Run("saga-3", s, SagaContext{}) // duplicate
	orch.Run("saga-3", s, SagaContext{}) // duplicate

	if calls != 1 {
		t.Errorf("idempotent step should run once, ran %d times", calls)
	}
}

// Test 12: Compensation event ordering in log.
func TestOrchestrator_CompensationEventOrdering(t *testing.T) {
	log := &SagaLog{}
	orch := NewSagaOrchestrator(log)

	s := &Saga{
		Steps: []Step{
			okStep("Step1"),
			okStep("Step2"),
			failStep("Step3", errors.New("step3 error")),
		},
	}

	orch.Run("saga-4", s, SagaContext{})

	events := log.EventsFor("saga-4")
	// Expected event sequence:
	// StepStarted/Step1, StepCompleted/Step1
	// StepStarted/Step2, StepCompleted/Step2
	// StepStarted/Step3, StepFailed/Step3
	// CompensationStarted/Step2, CompensationCompleted/Step2
	// CompensationStarted/Step1, CompensationCompleted/Step1
	expectedKinds := []EventKind{
		EventStepStarted, EventStepCompleted,
		EventStepStarted, EventStepCompleted,
		EventStepStarted, EventStepFailed,
		EventCompensationStarted, EventCompensationCompleted,
		EventCompensationStarted, EventCompensationCompleted,
	}

	if len(events) != len(expectedKinds) {
		t.Errorf("expected %d events, got %d:", len(expectedKinds), len(events))
		for i, e := range events {
			t.Logf("  [%d] %s", i, e)
		}
		return
	}
	for i, want := range expectedKinds {
		if events[i].Kind != want {
			t.Errorf("event[%d]: want %s, got %s", i, want, events[i].Kind)
		}
	}
}

// ── v2: Retry + HTTP participants ─────────────────────────────────────────────

// Test 13: Retry succeeds on the 3rd attempt.
func TestRetry_SucceedsOnThirdAttempt(t *testing.T) {
	var attempts int32

	s := Step{
		Name: "FlakyService",
		Execute: func(ctx SagaContext) error {
			n := atomic.AddInt32(&attempts, 1)
			if n < 3 {
				return &RetryableError{Msg: fmt.Sprintf("attempt %d failed", n)}
			}
			return nil
		},
		Compensate: func(ctx SagaContext) error { return nil },
	}

	policy := RetryPolicy{
		MaxAttempts:    3,
		BackoffBase:    1 * time.Millisecond,
		JitterFraction: 0,
	}
	retrying := WithRetry(s, policy)
	step := retrying.AsStep(context.Background())

	saga := &Saga{Steps: []Step{step}}
	result := saga.Run(SagaContext{})

	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted after 3 attempts, got %s: %v", result.Status, result.Error)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// Test 14: Budget exhaustion triggers compensation.
func TestRetry_BudgetExhaustionTriggersCompensation(t *testing.T) {
	var compensated bool

	step1 := Step{
		Name:       "PriorStep",
		Execute:    func(ctx SagaContext) error { return nil },
		Compensate: func(ctx SagaContext) error { compensated = true; return nil },
	}

	flakyStep := Step{
		Name:       "FlakyService",
		Execute:    func(ctx SagaContext) error { return &RetryableError{Msg: "always fails"} },
		Compensate: func(ctx SagaContext) error { return nil },
	}

	policy := RetryPolicy{
		MaxAttempts:    2,
		BackoffBase:    1 * time.Millisecond,
		JitterFraction: 0,
	}
	retrying := WithRetry(flakyStep, policy)
	retryStep := retrying.AsStep(context.Background())

	saga := &Saga{Steps: []Step{step1, retryStep}}
	result := saga.Run(SagaContext{})

	if result.Status != StatusCompensated {
		t.Errorf("expected StatusCompensated after exhausted budget, got %s", result.Status)
	}
	if !compensated {
		t.Error("expected PriorStep to be compensated after FlakyService exhausted retries")
	}
}

// Test 15: Non-retryable error skips retry.
func TestRetry_NonRetryableErrorSkipsRetry(t *testing.T) {
	var attempts int32

	step := Step{
		Name: "FatalService",
		Execute: func(ctx SagaContext) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("invalid input — fatal, do not retry")
		},
		Compensate: func(ctx SagaContext) error { return nil },
	}

	policy := RetryPolicy{
		MaxAttempts:    5,
		BackoffBase:    1 * time.Millisecond,
		JitterFraction: 0,
	}
	retrying := WithRetry(step, policy)
	retryStep := retrying.AsStep(context.Background())

	saga := &Saga{Steps: []Step{retryStep}}
	saga.Run(SagaContext{})

	if attempts != 1 {
		t.Errorf("non-retryable error should not retry, got %d attempts", attempts)
	}
}

// Test 16: Context cancellation aborts mid-retry.
func TestRetry_ContextCancellationAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var attempts int32
	step := Step{
		Name: "SlowService",
		Execute: func(sagaCtx SagaContext) error {
			n := atomic.AddInt32(&attempts, 1)
			if n == 1 {
				cancel() // cancel context after first attempt
			}
			return &RetryableError{Msg: "service slow"}
		},
		Compensate: func(sagaCtx SagaContext) error { return nil },
	}

	policy := RetryPolicy{
		MaxAttempts:    5,
		BackoffBase:    50 * time.Millisecond,
		JitterFraction: 0,
	}
	retrying := WithRetry(step, policy)
	retryStep := retrying.AsStep(ctx)

	saga := &Saga{Steps: []Step{retryStep}}
	result := saga.Run(SagaContext{})

	if result.Status != StatusCompensated {
		t.Errorf("expected StatusCompensated on context cancellation, got %s", result.Status)
	}
}

// Test 17: HTTP participant receives idempotency key in header.
func TestHttpStep_IdempotencyKeyInHeader(t *testing.T) {
	var receivedKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	step := HttpStep(HttpStepConfig{
		Name:             "InventoryReserve",
		Method:           "POST",
		URL:              server.URL + "/inventory/reserve",
		IdempotencyKeyFn: func(ctx SagaContext) string { return "saga-99/InventoryReserve" },
		Policy: RetryPolicy{
			MaxAttempts:    1,
			BackoffBase:    1 * time.Millisecond,
			JitterFraction: 0,
		},
		Timeout: 2 * time.Second,
	})

	saga := &Saga{Steps: []Step{step}}
	result := saga.Run(SagaContext{"sagaID": "saga-99"})

	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s: %v", result.Status, result.Error)
	}
	if receivedKey != "saga-99/InventoryReserve" {
		t.Errorf("expected idempotency key 'saga-99/InventoryReserve', got %q", receivedKey)
	}
}

// Test 18: HTTP participant 503 is retried, 400 is fatal.
func TestHttpStep_503IsRetried_400IsFatal(t *testing.T) {
	var callCount int32

	// Server returns 503 twice, then 200.
	server503 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server503.Close()

	step := HttpStep(HttpStepConfig{
		Name:   "PaymentCharge",
		Method: "POST",
		URL:    server503.URL + "/payment/charge",
		Policy: RetryPolicy{
			MaxAttempts:    3,
			BackoffBase:    1 * time.Millisecond,
			JitterFraction: 0,
		},
		Timeout: 2 * time.Second,
	})

	saga := &Saga{Steps: []Step{step}}
	result := saga.Run(SagaContext{"sagaID": "saga-100"})

	if result.Status != StatusCompleted {
		t.Errorf("expected 503 to be retried and succeed on 3rd attempt, got %s: %v", result.Status, result.Error)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries), got %d", callCount)
	}
}
