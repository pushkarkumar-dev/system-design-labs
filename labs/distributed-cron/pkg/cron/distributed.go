package cron

import (
	"context"
	"log"
	"sync"
	"time"
)

// DistributedScheduler wraps Scheduler with leader election and job history.
// Only the current leader executes jobs. Non-leaders skip silently.
// On leader transition, catchup fills in any missed runs.
type DistributedScheduler struct {
	nodeID      string
	elec        *LeaderElector
	store       *JobStore
	leaseTTL    time.Duration
	maxBackfill int

	mu      sync.Mutex
	entries []*distEntry
	stop    chan struct{}
	wg      sync.WaitGroup
}

// distEntry pairs a Job with its parsed schedule and next fire time.
type distEntry struct {
	job         *Job
	schedule    *CronSchedule
	next        time.Time
	lastLeaderAt time.Time // when this node last ran this job as leader
}

// NewDistributedScheduler creates a DistributedScheduler.
//
//   - nodeID: unique identifier for this process (e.g., hostname:port)
//   - elec: shared LeaderElector (same instance across all in-process nodes)
//   - store: shared JobStore for run history
//   - leaseTTL: how long the lease is valid; must be renewed before expiry
//   - maxBackfill: maximum missed runs to execute on leader transition
func NewDistributedScheduler(
	nodeID string,
	elec *LeaderElector,
	store *JobStore,
	leaseTTL time.Duration,
	maxBackfill int,
) *DistributedScheduler {
	return &DistributedScheduler{
		nodeID:      nodeID,
		elec:        elec,
		store:       store,
		leaseTTL:    leaseTTL,
		maxBackfill: maxBackfill,
		stop:        make(chan struct{}),
	}
}

// Add registers a job. Must be called before Start.
func (ds *DistributedScheduler) Add(j *Job) error {
	sched, err := Parse(j.CronExpr)
	if err != nil {
		return err
	}
	ds.mu.Lock()
	ds.entries = append(ds.entries, &distEntry{
		job:      j,
		schedule: sched,
		next:     sched.Next(time.Now()),
	})
	ds.mu.Unlock()
	return nil
}

// Start begins the scheduling loop and lease renewal loop.
func (ds *DistributedScheduler) Start() {
	ds.wg.Add(2)
	go ds.scheduleLoop()
	go ds.renewLoop()
}

// Stop shuts down the scheduler and waits for goroutines to exit.
func (ds *DistributedScheduler) Stop() {
	close(ds.stop)
	ds.wg.Wait()
}

// renewLoop periodically re-acquires or renews the leader lease.
// Renewal interval is leaseTTL/3 to ensure the lease never lapses while the
// node is healthy.
func (ds *DistributedScheduler) renewLoop() {
	defer ds.wg.Done()
	interval := ds.leaseTTL / 3
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ds.stop:
			return
		case <-ticker.C:
			ds.elec.Acquire(ds.nodeID, ds.leaseTTL)
		}
	}
}

// scheduleLoop wakes at each job's next fire time and executes it if this node
// is the current leader.
func (ds *DistributedScheduler) scheduleLoop() {
	defer ds.wg.Done()
	for {
		ds.mu.Lock()
		now := time.Now()
		var earliest time.Time
		for _, e := range ds.entries {
			if earliest.IsZero() || e.next.Before(earliest) {
				earliest = e.next
			}
		}
		ds.mu.Unlock()

		if earliest.IsZero() {
			select {
			case <-ds.stop:
				return
			case <-time.After(time.Minute):
			}
			continue
		}

		delay := earliest.Sub(now)
		if delay < 0 {
			delay = 0
		}

		timer := time.NewTimer(delay)
		select {
		case <-ds.stop:
			timer.Stop()
			return
		case fired := <-timer.C:
			isLeader := ds.elec.IsLeader(ds.nodeID)
			ds.mu.Lock()
			for _, e := range ds.entries {
				if !e.next.After(fired) {
					scheduledAt := e.next
					e.next = e.schedule.Next(e.next)
					if isLeader {
						ds.wg.Add(1)
						isFirstLeaderRun := e.lastLeaderAt.IsZero()
						e.lastLeaderAt = scheduledAt
						go func(entry *distEntry, t time.Time, firstRun bool) {
							defer ds.wg.Done()
							ctx, cancel := context.WithCancel(context.Background())
							defer cancel()
							// On first run as leader, backfill any missed runs.
							if firstRun && ds.maxBackfill > 0 {
								since := t.Add(-7 * 24 * time.Hour) // 7-day window
								Catchup(ctx, entry.job, entry.schedule, ds.store, since, ds.maxBackfill)
							}
							ExecWithHistory(ctx, entry.job, t, ds.store)
						}(e, scheduledAt, isFirstLeaderRun)
					} else {
						log.Printf("cron: node %q is not leader — skipping job %q (scheduled %s)",
							ds.nodeID, e.job.Name, scheduledAt.Format(time.RFC3339))
					}
				}
			}
			ds.mu.Unlock()
		}
	}
}

// IsLeader returns whether this node currently holds the leader lease.
func (ds *DistributedScheduler) IsLeader() bool {
	return ds.elec.IsLeader(ds.nodeID)
}
