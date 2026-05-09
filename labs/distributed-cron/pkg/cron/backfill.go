package cron

import (
	"context"
	"log"
	"time"
)

// Catchup finds all scheduled fire times for job between since and now
// that have no JobRun record in store, and executes them synchronously
// (in fire-time order) up to maxBackfill runs.
//
// This is called on leader election to recover jobs that were missed while
// no leader was running (e.g., after a crash or during a rolling restart).
// Exported for testing; internal callers use the lowercase alias below.
func Catchup(ctx context.Context, job *Job, schedule *CronSchedule, store *JobStore, since time.Time, maxBackfill int) {
	if maxBackfill <= 0 {
		return
	}
	now := time.Now()

	// Collect all scheduled fire times in [since, now].
	var missed []time.Time
	t := since
	for {
		t = schedule.Next(t)
		if t.IsZero() || !t.Before(now) {
			break
		}
		if !store.HasRunAt(job.Name, t) {
			missed = append(missed, t)
		}
	}

	if len(missed) == 0 {
		return
	}

	totalMissed := len(missed)
	// Cap at maxBackfill — take the most recent missed runs.
	if len(missed) > maxBackfill {
		missed = missed[len(missed)-maxBackfill:]
	}

	log.Printf("cron: backfill job %q: executing %d missed run(s) (of %d total missed)", job.Name, len(missed), totalMissed)

	for _, scheduledAt := range missed {
		select {
		case <-ctx.Done():
			return
		default:
		}
		ExecWithHistory(ctx, job, scheduledAt, store)
	}
}

// ExecWithHistory runs a single job execution and records it in the store.
// This is the canonical execution path for both backfill and normal scheduled runs.
// Exported for testing; use the lowercase alias inside the package.
func ExecWithHistory(ctx context.Context, job *Job, scheduledAt time.Time, store *JobStore) {
	startedAt := time.Now()
	run := JobRun{
		JobName:     job.Name,
		ScheduledAt: scheduledAt,
		StartedAt:   startedAt,
		Status:      StatusRunning,
	}
	store.Append(run)

	jobCtx, cancel := context.WithTimeout(ctx, job.timeout())
	defer cancel()

	err := job.Fn(jobCtx)

	status := StatusCompleted
	errMsg := ""
	if err != nil {
		if jobCtx.Err() != nil {
			status = StatusTimeout
		} else {
			status = StatusFailed
		}
		errMsg = err.Error()
	}
	store.UpdateStatus(job.Name, scheduledAt, status, errMsg, time.Now())
}
