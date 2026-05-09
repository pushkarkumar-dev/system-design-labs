package dev.pushkar.cron;

import net.javacrumbs.shedlock.spring.annotation.SchedulerLock;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

/**
 * Demonstrates two approaches to distributed cron in Spring Boot.
 *
 * APPROACH 1 — Plain @Scheduled (WRONG for distributed systems):
 *   Every pod in a 3-replica deployment fires minutelyReportUnsafe() every minute.
 *   If each run inserts a DB row or calls an external API, you get 3x the work.
 *   This is fine for cache warming, but wrong for billing, report generation, etc.
 *
 * APPROACH 2 — @Scheduled + @SchedulerLock (CORRECT):
 *   ShedLock wraps the method with a Redis SET NX PX — a distributed compare-and-swap
 *   identical in concept to our Go lab's LeaderElector.Acquire(nodeID, ttl).
 *   Only the pod that wins the CAS executes the method body. The others skip silently.
 *
 * ShedLock parameters:
 *   lockAtMostFor  = maximum lease TTL (protects against crashed pod holding the lock forever)
 *   lockAtLeastFor = minimum lock hold time (prevents re-execution if the job finishes in <1s)
 */
@Service
public class CronJobService {

    private static final Logger log = LoggerFactory.getLogger(CronJobService.class);

    // -------------------------------------------------------------------------
    // Approach 1: plain @Scheduled — fires on EVERY pod (unsafe for distributed)
    // -------------------------------------------------------------------------

    /**
     * Runs every minute on every pod. Safe for idempotent, read-only work
     * (e.g., refreshing an in-process cache). Dangerous for writes.
     */
    @Scheduled(cron = "0 * * * * *") // Spring cron: second minute hour dom month dow
    public void minutelyReportUnsafe() {
        log.warn("UNSAFE: minutelyReport fired — this runs on every pod in the cluster");
        // In a 3-replica deployment this executes 3 times per minute.
    }

    // -------------------------------------------------------------------------
    // Approach 2: @Scheduled + @SchedulerLock — fires on EXACTLY ONE pod
    // -------------------------------------------------------------------------

    /**
     * Runs every minute but only on the pod that wins the Redis lease.
     * ShedLock key: "minutelyReport" stored in Redis with TTL = lockAtMostFor.
     *
     * This mirrors our Go lab's DistributedScheduler.scheduleLoop():
     *   if !ds.elec.IsLeader(ds.nodeID) { skip }
     */
    @Scheduled(cron = "0 * * * * *")
    @SchedulerLock(name = "minutelyReport", lockAtMostFor = "PT2M", lockAtLeastFor = "PT55S")
    public void minutelyReport() {
        log.info("minutelyReport: running on exactly one pod (ShedLock lease acquired)");
        // Simulate report generation work.
        try {
            Thread.sleep(100);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        log.info("minutelyReport: done");
    }

    /**
     * Weekly digest: fires at 9:00am every Monday.
     * lockAtMostFor = 10 minutes — if the pod crashes mid-run, another pod
     * can take over after at most 10 minutes (same as our LeaderLease TTL).
     */
    @Scheduled(cron = "0 0 9 * * MON")
    @SchedulerLock(name = "weeklyReport", lockAtMostFor = "PT10M", lockAtLeastFor = "PT5M")
    public void weeklyReport() {
        log.info("weeklyReport: generating weekly digest on exactly one pod");
        // In production: query DB, build report, send email.
        log.info("weeklyReport: complete");
    }
}
