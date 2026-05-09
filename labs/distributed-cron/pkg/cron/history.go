package cron

import (
	"sync"
	"time"
)

// JobStatus values for a JobRun.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusTimeout   = "timeout"
)

// JobRun is a single execution record for a job.
type JobRun struct {
	JobName     string
	ScheduledAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	Status      string
	Error       string
}

// JobStore is an in-memory store of JobRun history keyed by job name.
// All methods are safe for concurrent use.
type JobStore struct {
	mu   sync.RWMutex
	runs map[string][]JobRun
}

// NewJobStore returns an empty JobStore.
func NewJobStore() *JobStore {
	return &JobStore{runs: make(map[string][]JobRun)}
}

// Append adds a run record to the history for jobName.
func (s *JobStore) Append(run JobRun) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.JobName] = append(s.runs[run.JobName], run)
}

// RunsFor returns a copy of all run records for jobName.
func (s *JobStore) RunsFor(jobName string) []JobRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.runs[jobName]
	if len(src) == 0 {
		return nil
	}
	out := make([]JobRun, len(src))
	copy(out, src)
	return out
}

// HasRunAt returns true if there is a completed or running record for jobName
// with ScheduledAt equal to t (within 1-second tolerance).
func (s *JobStore) HasRunAt(jobName string, t time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.runs[jobName] {
		if r.ScheduledAt.Equal(t) {
			return true
		}
	}
	return false
}

// UpdateStatus updates the Status and FinishedAt of the most-recent run
// for jobName that matches scheduledAt.
func (s *JobStore) UpdateStatus(jobName string, scheduledAt time.Time, status, errMsg string, finishedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runs := s.runs[jobName]
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].ScheduledAt.Equal(scheduledAt) {
			runs[i].Status = status
			runs[i].Error = errMsg
			runs[i].FinishedAt = finishedAt
			s.runs[jobName] = runs
			return
		}
	}
}
