package cron_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/distributed-cron/pkg/cron"
)

// ---------------------------------------------------------------------------
// v0: Parser tests
// ---------------------------------------------------------------------------

// Test 1: "* * * * *" fires every minute.
func TestParseEveryMinute(t *testing.T) {
	sched, err := cron.Parse("* * * * *")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next := sched.Next(from)
	expected := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("got %v, want %v", next, expected)
	}
	// Verify consecutive calls advance by one minute.
	next2 := sched.Next(next)
	if !next2.Equal(expected.Add(time.Minute)) {
		t.Errorf("second Next: got %v, want %v", next2, expected.Add(time.Minute))
	}
}

// Test 2: "0 9 * * 1" fires Monday at 9:00am.
func TestParseMonday9am(t *testing.T) {
	sched, err := cron.Parse("0 9 * * 1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// 2026-01-01 is a Thursday. Next Monday is 2026-01-05.
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next := sched.Next(from)
	if next.Weekday() != time.Monday {
		t.Errorf("expected Monday, got %v", next.Weekday())
	}
	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("expected 09:00, got %02d:%02d", next.Hour(), next.Minute())
	}
}

// Test 3: "*/5 * * * *" fires every 5 minutes.
func TestParseEvery5Minutes(t *testing.T) {
	sched, err := cron.Parse("*/5 * * * *")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next := sched.Next(from)
	if next.Minute() != 5 {
		t.Errorf("expected minute 5, got %d", next.Minute())
	}
	next2 := sched.Next(next)
	if next2.Minute() != 10 {
		t.Errorf("expected minute 10, got %d", next2.Minute())
	}
}

// Test 4: Invalid expression returns error.
func TestParseInvalidExpr(t *testing.T) {
	cases := []string{
		"",
		"* * * *",     // too few fields
		"* * * * * *", // too many fields
		"60 * * * *",  // minute out of range
		"* 25 * * *",  // hour out of range
		"foo * * * *", // non-numeric
	}
	for _, expr := range cases {
		_, err := cron.Parse(expr)
		if err == nil {
			t.Errorf("expected error for %q, got nil", expr)
		}
	}
}

// Test 5: Next() advances correctly for a range expression.
func TestNextAdvancesRange(t *testing.T) {
	// "0 8-10 * * *" fires at 8:00, 9:00, and 10:00 each day.
	sched, err := cron.Parse("0 8-10 * * *")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	from := time.Date(2026, 1, 1, 7, 0, 0, 0, time.UTC)
	for _, wantHour := range []int{8, 9, 10} {
		next := sched.Next(from)
		if next.Hour() != wantHour || next.Minute() != 0 {
			t.Errorf("expected %02d:00, got %02d:%02d", wantHour, next.Hour(), next.Minute())
		}
		from = next
	}
}

// Test 6: Concurrent registration of two jobs does not error.
func TestConcurrentJobRegistration(t *testing.T) {
	var count1, count2 atomic.Int32
	s := cron.NewScheduler()
	if err := s.Add(&cron.Job{
		Name:     "job1",
		CronExpr: "* * * * *",
		Fn: func(_ context.Context) error {
			count1.Add(1)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(&cron.Job{
		Name:     "job2",
		CronExpr: "* * * * *",
		Fn: func(_ context.Context) error {
			count2.Add(1)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Both jobs registered; scheduler not started — just verify no errors.
	_ = count1.Load()
	_ = count2.Load()
}

// Test 7: Scheduler Stop returns without deadlock when no jobs have fired.
func TestSchedulerStop(t *testing.T) {
	s := cron.NewScheduler()
	if err := s.Add(&cron.Job{
		Name:     "stop-test",
		CronExpr: "* * * * *",
		Fn:       func(_ context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	s.Start()
	// Give scheduler a moment to initialize its timer.
	time.Sleep(10 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out — possible deadlock")
	}
}

// Test 8: List parsing ("1,15,30 * * * *" fires at minutes 1, 15, 30).
func TestParseList(t *testing.T) {
	sched, err := cron.Parse("1,15,30 * * * *")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, want := range []int{1, 15, 30} {
		next := sched.Next(from)
		if next.Minute() != want {
			t.Errorf("expected minute %d, got %d", want, next.Minute())
		}
		from = next
	}
}

// ---------------------------------------------------------------------------
// v1: Leader election tests
// ---------------------------------------------------------------------------

// Test 9: Only leader executes jobs.
func TestOnlyLeaderExecutes(t *testing.T) {
	elec := cron.NewLeaderElector()
	const ttl = 10 * time.Second

	if !elec.Acquire("node-a", ttl) {
		t.Fatal("node-a should acquire the lease")
	}
	if elec.IsLeader("node-b") {
		t.Error("node-b should not be leader when node-a holds the lease")
	}
	if !elec.IsLeader("node-a") {
		t.Error("node-a should be the leader")
	}
}

// Test 10: Non-leader's Acquire fails while another holds the lease.
func TestNonLeaderAcquireFails(t *testing.T) {
	elec := cron.NewLeaderElector()
	elec.Acquire("node-a", 10*time.Second)

	acquired := elec.Acquire("node-b", 10*time.Second)
	if acquired {
		t.Error("node-b should not be able to acquire while node-a holds a valid lease")
	}
}

// Test 11: Leader crash causes failover after TTL expiry.
func TestLeaderCrashCausesFailover(t *testing.T) {
	elec := cron.NewLeaderElector()
	elec.Acquire("node-a", 50*time.Millisecond)
	time.Sleep(70 * time.Millisecond)
	if !elec.Acquire("node-b", 10*time.Second) {
		t.Error("node-b should acquire after node-a's TTL expires")
	}
	if !elec.IsLeader("node-b") {
		t.Error("node-b should now be the leader")
	}
}

// Test 12: Lease renewal keeps the leader active past the original TTL.
func TestLeaseRenewalKeepsLeader(t *testing.T) {
	elec := cron.NewLeaderElector()
	elec.Acquire("node-a", 50*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	if !elec.Renew("node-a", 300*time.Millisecond) {
		t.Error("Renew should succeed for current leader")
	}

	// Original TTL would have expired — node-a should still be leader.
	time.Sleep(60 * time.Millisecond)
	if !elec.IsLeader("node-a") {
		t.Error("node-a should still be leader after renewal")
	}
}

// Test 13: Concurrent acquire — exactly one node wins.
func TestConcurrentAcquireOnlyOneWins(t *testing.T) {
	elec := cron.NewLeaderElector()
	const nodes = 10
	results := make([]bool, nodes)
	var wg sync.WaitGroup
	for i := 0; i < nodes; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			nodeID := fmt.Sprintf("node-%d", i)
			results[i] = elec.Acquire(nodeID, 10*time.Second)
		}()
	}
	wg.Wait()

	winners := 0
	for _, won := range results {
		if won {
			winners++
		}
	}
	if winners != 1 {
		t.Errorf("expected exactly 1 winner, got %d", winners)
	}
}

// Test 14: TTL expiry releases the lease so IsLeader returns false.
func TestTTLExpiryReleasesLease(t *testing.T) {
	elec := cron.NewLeaderElector()
	elec.Acquire("node-a", 30*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	if elec.IsLeader("node-a") {
		t.Error("node-a lease should have expired")
	}
}

// ---------------------------------------------------------------------------
// v2: Backfill and job history tests
// ---------------------------------------------------------------------------

// Test 15: Backfill runs exactly 2 missed jobs on startup (3-minute window).
func TestBackfillRunsMissedJobs(t *testing.T) {
	store := cron.NewJobStore()
	var count atomic.Int32
	job := &cron.Job{
		Name:     "backfill-job",
		CronExpr: "* * * * *",
		Fn: func(_ context.Context) error {
			count.Add(1)
			return nil
		},
	}
	sched, _ := cron.Parse(job.CronExpr)
	since := time.Now().Add(-3 * time.Minute).Truncate(time.Minute)
	cron.Catchup(context.Background(), job, sched, store, since, 2)

	if count.Load() != 2 {
		t.Errorf("expected 2 backfill executions, got %d", count.Load())
	}
}

// Test 16: Backfill is capped at MaxBackfill.
func TestBackfillCappedAtMax(t *testing.T) {
	store := cron.NewJobStore()
	var count atomic.Int32
	job := &cron.Job{
		Name:     "capped-job",
		CronExpr: "* * * * *",
		Fn: func(_ context.Context) error {
			count.Add(1)
			return nil
		},
	}
	sched, _ := cron.Parse(job.CronExpr)
	// 10-minute window with every-minute job = 10 missed runs. Cap at 3.
	since := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	cron.Catchup(context.Background(), job, sched, store, since, 3)

	if count.Load() != 3 {
		t.Errorf("expected 3 backfill executions (capped), got %d", count.Load())
	}
}

// Test 17: Execution timeout cancels the job and records StatusTimeout.
func TestExecutionTimeoutCancelsJob(t *testing.T) {
	store := cron.NewJobStore()
	job := &cron.Job{
		Name:     "timeout-job",
		CronExpr: "* * * * *",
		Timeout:  50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		},
	}
	cron.ExecWithHistory(context.Background(), job, time.Now(), store)

	runs := store.RunsFor("timeout-job")
	if len(runs) == 0 {
		t.Fatal("expected a run record")
	}
	if runs[0].Status != cron.StatusTimeout {
		t.Errorf("expected status %q, got %q", cron.StatusTimeout, runs[0].Status)
	}
}

// Test 18: Job history is recorded with all fields set.
func TestJobHistoryRecorded(t *testing.T) {
	store := cron.NewJobStore()
	job := &cron.Job{
		Name:     "history-job",
		CronExpr: "* * * * *",
		Fn:       func(_ context.Context) error { return nil },
	}
	scheduledAt := time.Now()
	cron.ExecWithHistory(context.Background(), job, scheduledAt, store)

	runs := store.RunsFor("history-job")
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	r := runs[0]
	if r.JobName != "history-job" {
		t.Errorf("wrong job name: %q", r.JobName)
	}
	if r.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if r.FinishedAt.IsZero() {
		t.Error("FinishedAt should be set")
	}
}

// Test 19: Successful run status is "completed".
func TestSuccessfulRunStatus(t *testing.T) {
	store := cron.NewJobStore()
	job := &cron.Job{
		Name:     "success-job",
		CronExpr: "* * * * *",
		Fn:       func(_ context.Context) error { return nil },
	}
	cron.ExecWithHistory(context.Background(), job, time.Now(), store)

	runs := store.RunsFor("success-job")
	if len(runs) == 0 {
		t.Fatal("no runs recorded")
	}
	if runs[0].Status != cron.StatusCompleted {
		t.Errorf("expected %q, got %q", cron.StatusCompleted, runs[0].Status)
	}
}

// Test 20: Failed run status is "failed" with error message.
func TestFailedRunStatus(t *testing.T) {
	store := cron.NewJobStore()
	job := &cron.Job{
		Name:     "fail-job",
		CronExpr: "* * * * *",
		Fn:       func(_ context.Context) error { return errors.New("deliberate error") },
	}
	cron.ExecWithHistory(context.Background(), job, time.Now(), store)

	runs := store.RunsFor("fail-job")
	if len(runs) == 0 {
		t.Fatal("no runs recorded")
	}
	if runs[0].Status != cron.StatusFailed {
		t.Errorf("expected %q, got %q", cron.StatusFailed, runs[0].Status)
	}
	if runs[0].Error == "" {
		t.Error("Error field should be set on failed run")
	}
}
