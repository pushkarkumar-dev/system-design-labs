// Package cron implements a distributed cron scheduler in three stages.
//
// v0: Cron expression parser + local scheduler
//   - 5-field cron expressions: minute, hour, day-of-month, month, day-of-week
//   - Supports *, n, */n, n-m, n,m field syntax
//   - CronSchedule.Next(from) returns next fire time after from
//   - Scheduler runs jobs in separate goroutines at their scheduled times
//
// v1: Leader election for exactly-once execution
//   - LeaderElector uses atomic CAS on a shared LeaderLease
//   - Acquire(nodeID, ttl) — returns true if this node holds the lease
//   - Renew(nodeID) — extends the lease TTL if still held by nodeID
//   - DistributedScheduler wraps Scheduler; non-leaders skip job execution
//
// v2: Job history + missed-job backfill
//   - JobRun records scheduled time, start, finish, status, and error
//   - JobStore keeps an in-memory run history per job
//   - catchup(job, since) finds and re-runs missed scheduled times
//   - MaxBackfill caps the number of missed runs executed on startup
package cron
