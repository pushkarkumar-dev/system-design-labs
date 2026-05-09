// Package main is a demo server for the distributed-cron lab.
// It starts a DistributedScheduler with two example jobs and prints
// run history to stdout every 10 seconds.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pushkar1005/system-design-labs/labs/distributed-cron/pkg/cron"
)

func main() {
	// In a real deployment, nodeID would be the pod name or hostname.
	nodeID := fmt.Sprintf("node-%d", os.Getpid())
	log.Printf("distributed-cron demo: starting node %q", nodeID)

	elec := cron.NewLeaderElector()
	store := cron.NewJobStore()

	const leaseTTL = 30 * time.Second
	const maxBackfill = 3

	ds := cron.NewDistributedScheduler(nodeID, elec, store, leaseTTL, maxBackfill)

	// Job 1: runs every minute, simulates a lightweight report generation.
	if err := ds.Add(&cron.Job{
		Name:     "minutely-report",
		CronExpr: "* * * * *",
		Timeout:  2 * time.Minute,
		Fn: func(ctx context.Context) error {
			log.Printf("[job] minutely-report: generating report")
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
			log.Printf("[job] minutely-report: done")
			return nil
		},
	}); err != nil {
		log.Fatalf("Add minutely-report: %v", err)
	}

	// Job 2: runs at 9am Monday — simulates a weekly digest.
	if err := ds.Add(&cron.Job{
		Name:     "weekly-digest",
		CronExpr: "0 9 * * 1",
		Timeout:  10 * time.Minute,
		Fn: func(ctx context.Context) error {
			log.Printf("[job] weekly-digest: sending digest")
			return nil
		},
	}); err != nil {
		log.Fatalf("Add weekly-digest: %v", err)
	}

	// Attempt initial lease acquisition.
	if elec.Acquire(nodeID, leaseTTL) {
		log.Printf("node %q acquired leader lease", nodeID)
	}

	ds.Start()

	// Print run history every 10 seconds.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runs := store.RunsFor("minutely-report")
			log.Printf("job history: minutely-report has %d run(s)", len(runs))
			for _, r := range runs {
				log.Printf("  run scheduled=%s status=%s", r.ScheduledAt.Format(time.RFC3339), r.Status)
			}
		}
	}()

	// Wait for SIGINT or SIGTERM.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Printf("shutting down node %q", nodeID)
	ds.Stop()
	log.Printf("node %q stopped cleanly", nodeID)
}
