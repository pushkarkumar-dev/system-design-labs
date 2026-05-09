package saga

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ── HttpStep ──────────────────────────────────────────────────────────────────

// HttpStepConfig configures a step that calls a participant service over HTTP.
// The step sends a POST request with an idempotency key header so the
// participant can detect and deduplicate retried requests.
type HttpStepConfig struct {
	// Name is the step name (used in logs and results).
	Name string
	// Method is the HTTP method (e.g., "POST", "DELETE").
	Method string
	// URL is the participant service endpoint.
	URL string
	// IdempotencyKeyFn extracts the idempotency key from the SagaContext.
	// Defaults to the saga ID if nil.
	IdempotencyKeyFn func(ctx SagaContext) string
	// BodyFn builds the request body from the SagaContext.
	// Returns empty string if nil (no body).
	BodyFn func(ctx SagaContext) string
	// SuccessKey is the SagaContext key to store the response body under.
	// Not stored if empty.
	SuccessKey string
	// Timeout is the per-request HTTP timeout. Defaults to 5s.
	Timeout time.Duration
	// Policy is the retry policy. Defaults to DefaultRetryPolicy.
	Policy RetryPolicy

	// CompensateMethod is the HTTP method for the compensation call (e.g., "DELETE").
	CompensateMethod string
	// CompensateURL is the endpoint to call for compensation. If empty, compensation is a no-op.
	CompensateURL string
}

// HttpStep builds a Step that calls a participant service over HTTP.
// The step is automatically wrapped with the retry policy.
// A non-2xx response is treated as a RetryableError (503) or fatal (4xx).
func HttpStep(cfg HttpStepConfig) Step {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	policy := cfg.Policy
	if policy.MaxAttempts == 0 {
		policy = DefaultRetryPolicy
	}

	execute := func(ctx SagaContext) error {
		idempotencyKey := ""
		if cfg.IdempotencyKeyFn != nil {
			idempotencyKey = cfg.IdempotencyKeyFn(ctx)
		} else {
			idempotencyKey = ctx.GetString("sagaID") + "/" + cfg.Name
		}

		body := ""
		if cfg.BodyFn != nil {
			body = cfg.BodyFn(ctx)
		}

		hctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(hctx, cfg.Method, cfg.URL, strings.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			// Network error — retryable.
			return &RetryableError{Cause: err, Msg: "HTTP request failed"}
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if cfg.SuccessKey != "" {
				ctx.Set(cfg.SuccessKey, string(respBody))
			}
			return nil
		case resp.StatusCode == 429 || resp.StatusCode == 503 || resp.StatusCode == 504:
			// Transient server errors — retryable.
			return &RetryableError{
				Msg: fmt.Sprintf("participant returned %d: %s", resp.StatusCode, string(respBody)),
			}
		default:
			// 4xx or other 5xx — fatal, trigger compensation.
			return fmt.Errorf("participant returned %d: %s", resp.StatusCode, string(respBody))
		}
	}

	compensate := func(ctx SagaContext) error {
		if cfg.CompensateURL == "" {
			return nil // no compensation needed for this participant
		}

		method := cfg.CompensateMethod
		if method == "" {
			method = "DELETE"
		}

		hctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := http.NewRequestWithContext(hctx, method, cfg.CompensateURL, nil)
		if err != nil {
			return fmt.Errorf("build compensation request: %w", err)
		}

		idempotencyKey := ""
		if cfg.IdempotencyKeyFn != nil {
			idempotencyKey = cfg.IdempotencyKeyFn(ctx)
		} else {
			idempotencyKey = ctx.GetString("sagaID") + "/" + cfg.Name + "/compensate"
		}
		req.Header.Set("Idempotency-Key", idempotencyKey+"/compensate")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("compensation HTTP call failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("compensation returned %d: %s", resp.StatusCode, string(body))
	}

	// Wrap execute with retry policy.
	retrying := WithRetry(Step{Name: cfg.Name, Execute: execute, Compensate: compensate}, policy)
	bgCtx := context.Background()
	return retrying.AsStep(bgCtx)
}
