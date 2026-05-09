package cron

import (
	"context"
	"log"
	"sync"
	"time"
)

// Job is a named unit of work with a cron expression and an optional timeout.
type Job struct {
	Name     string
	CronExpr string
	Timeout  time.Duration // 0 means no timeout (default 5 minutes)
	Fn       func(ctx context.Context) error
}

// timeout returns the effective timeout for this job.
func (j *Job) timeout() time.Duration {
	if j.Timeout > 0 {
		return j.Timeout
	}
	return 5 * time.Minute
}

// Scheduler runs a set of jobs on their cron schedules.
// Each job fires in its own goroutine. Scheduler itself is one goroutine
// that wakes at each job's next tick.
type Scheduler struct {
	jobs     []*jobEntry
	mu       sync.Mutex
	stop     chan struct{}
	wg       sync.WaitGroup
}

// jobEntry pairs a Job with its parsed schedule and next scheduled time.
type jobEntry struct {
	job      *Job
	schedule *CronSchedule
	next     time.Time
}

// NewScheduler creates a Scheduler with no jobs. Call Add before Start.
func NewScheduler() *Scheduler {
	return &Scheduler{
		stop: make(chan struct{}),
	}
}

// Add registers a job with the scheduler. It must be called before Start.
// Returns an error if the job's cron expression is invalid.
func (s *Scheduler) Add(j *Job) error {
	sched, err := Parse(j.CronExpr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.jobs = append(s.jobs, &jobEntry{
		job:      j,
		schedule: sched,
		next:     sched.Next(time.Now()),
	})
	s.mu.Unlock()
	return nil
}

// Start begins the scheduling loop in a background goroutine.
// Call Stop to shut down cleanly.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop signals the scheduler to halt and waits for in-flight runs to complete.
func (s *Scheduler) Stop() {
	close(s.stop)
	s.wg.Wait()
}

// run is the main scheduling loop. It sleeps until the next job is due,
// launches it, then recalculates the next fire time.
func (s *Scheduler) run() {
	defer s.wg.Done()
	for {
		s.mu.Lock()
		now := time.Now()
		var earliest time.Time
		for _, e := range s.jobs {
			if earliest.IsZero() || e.next.Before(earliest) {
				earliest = e.next
			}
		}
		s.mu.Unlock()

		if earliest.IsZero() {
			// No jobs registered — idle.
			select {
			case <-s.stop:
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
		case <-s.stop:
			timer.Stop()
			return
		case fired := <-timer.C:
			s.mu.Lock()
			for _, e := range s.jobs {
				if !e.next.After(fired) {
					s.runJob(e, e.next)
					e.next = e.schedule.Next(e.next)
				}
			}
			s.mu.Unlock()
		}
	}
}

// runJob launches a single job execution in a goroutine with a timeout context.
// Override this in DistributedScheduler to add leader-election gating.
func (s *Scheduler) runJob(e *jobEntry, scheduledAt time.Time) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), e.job.timeout())
		defer cancel()
		if err := e.job.Fn(ctx); err != nil {
			log.Printf("cron: job %q (scheduled %s) error: %v", e.job.Name, scheduledAt.Format(time.RFC3339), err)
		}
	}()
}
