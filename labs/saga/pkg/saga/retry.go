package saga

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// ── RetryableError ───────────────────────────────────────────────────────────

// RetryableError wraps an error to signal that the step should be retried
// instead of immediately triggering compensation. Steps return this when
// the failure is transient (network timeout, service unavailable) rather than
// permanent (invalid input, business rule violation).
//
// Example:
//
//	func callPaymentService(ctx SagaContext) error {
//	    resp, err := http.Post(...)
//	    if err != nil || resp.StatusCode == 503 {
//	        return &RetryableError{Cause: err, Msg: "payment service unavailable"}
//	    }
//	    if resp.StatusCode == 400 {
//	        return fmt.Errorf("invalid payment data") // fatal — compensate immediately
//	    }
//	    return nil
//	}
type RetryableError struct {
	Cause error
	Msg   string
}

func (e *RetryableError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("retryable: %s: %v", e.Msg, e.Cause)
	}
	return fmt.Sprintf("retryable: %s", e.Msg)
}

func (e *RetryableError) Unwrap() error { return e.Cause }

// IsRetryable returns true if the error is (or wraps) a RetryableError.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Walk the error chain looking for *RetryableError.
	for e := err; e != nil; {
		if _, ok := e.(*RetryableError); ok {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			break
		}
		e = u.Unwrap()
	}
	return false
}

// ── RetryPolicy ──────────────────────────────────────────────────────────────

// RetryPolicy configures how a step is retried on RetryableError.
// Non-retryable errors bypass the policy and trigger compensation immediately.
type RetryPolicy struct {
	// MaxAttempts is the total number of attempts (1 = no retry).
	MaxAttempts int
	// BackoffBase is the initial backoff duration before the first retry.
	BackoffBase time.Duration
	// MaxBackoff caps the exponential growth. Zero means no cap.
	MaxBackoff time.Duration
	// JitterFraction adds random jitter in [0, BackoffBase*JitterFraction].
	// 0.2 adds up to 20% jitter to prevent thundering herds.
	JitterFraction float64
}

// DefaultRetryPolicy is a sensible default: 3 attempts, 100ms base, 5s cap, 20% jitter.
var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts:    3,
	BackoffBase:    100 * time.Millisecond,
	MaxBackoff:     5 * time.Second,
	JitterFraction: 0.2,
}

// backoffDuration computes the wait before attempt number `attempt` (1-indexed).
// Uses exponential backoff: base * 2^(attempt-1), capped at MaxBackoff, with jitter.
func (p RetryPolicy) backoffDuration(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	exp := math.Pow(2, float64(attempt-2)) // attempt=2 -> 2^0=1, attempt=3 -> 2^1=2, ...
	d := time.Duration(float64(p.BackoffBase) * exp)
	if p.MaxBackoff > 0 && d > p.MaxBackoff {
		d = p.MaxBackoff
	}
	// Add jitter: random fraction of BackoffBase.
	if p.JitterFraction > 0 {
		jitter := time.Duration(rand.Float64() * float64(p.BackoffBase) * p.JitterFraction)
		d += jitter
	}
	return d
}

// ── RetryingStep ─────────────────────────────────────────────────────────────

// RetryingStep wraps a Step with a RetryPolicy. On RetryableError, it retries
// up to MaxAttempts times with exponential backoff. On a fatal error or when
// the context is cancelled (saga timeout), it returns immediately.
type RetryingStep struct {
	Step
	Policy RetryPolicy
}

// WithRetry wraps a Step with the given RetryPolicy.
func WithRetry(s Step, p RetryPolicy) RetryingStep {
	return RetryingStep{Step: s, Policy: p}
}

// AsStep converts a RetryingStep back to a plain Step with retry logic embedded
// in the Execute function. The Compensate function is unchanged.
func (rs RetryingStep) AsStep(sagaCtx context.Context) Step {
	return Step{
		Name: rs.Name,
		Execute: func(ctx SagaContext) error {
			var lastErr error
			for attempt := 1; attempt <= rs.Policy.MaxAttempts; attempt++ {
				// Check for saga-level timeout before attempting.
				select {
				case <-sagaCtx.Done():
					return fmt.Errorf("saga context cancelled before attempt %d: %w", attempt, sagaCtx.Err())
				default:
				}

				if attempt > 1 {
					backoff := rs.Policy.backoffDuration(attempt)
					select {
					case <-sagaCtx.Done():
						return fmt.Errorf("saga context cancelled during backoff: %w", sagaCtx.Err())
					case <-time.After(backoff):
					}
				}

				err := rs.Step.Execute(ctx)
				if err == nil {
					return nil
				}

				if !IsRetryable(err) {
					// Fatal error — do not retry, trigger compensation.
					return err
				}

				lastErr = err
				// Continue to next attempt.
			}
			// Exhausted all attempts.
			return fmt.Errorf("exhausted %d attempts: %w", rs.Policy.MaxAttempts, lastErr)
		},
		Compensate: rs.Compensate,
	}
}
