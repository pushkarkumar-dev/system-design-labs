// Package saga implements a saga orchestrator in three progressive stages.
//
// Three versions live in this package:
//
//	v0 — In-memory saga runner. A Saga is an ordered list of Steps. Each Step
//	     has an Execute function and a Compensate function. Run() executes steps
//	     forward; on first failure, runs compensations in reverse order.
//	     Key lesson: compensation ordering is strict — compensate in reverse to
//	     respect the dependencies between steps.
//
//	v1 — Persistent event log + idempotency. SagaOrchestrator writes events
//	     (StepStarted, StepCompleted, StepFailed, CompensationStarted,
//	     CompensationCompleted, CompensationFailed) to an append-only SagaLog.
//	     Recover(sagaID) replays the log to resume from the correct position.
//	     Key lesson: the event log is what makes saga recovery deterministic.
//
//	v2 — Retry budgets + HTTP participants. Steps declare RetryableError to
//	     trigger exponential backoff with jitter. A saga-level context timeout
//	     aborts and compensates even mid-retry. HttpStep builds a step that
//	     calls a participant service over HTTP with an idempotency key header.
//	     Key lesson: retries and compensations are orthogonal — retry buys time
//	     before giving up; compensation cleans up after giving up.
package saga
