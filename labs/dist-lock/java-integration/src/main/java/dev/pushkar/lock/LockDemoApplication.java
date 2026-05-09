package dev.pushkar.lock;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

/**
 * Demonstrates the @DistributedLock AOP aspect.
 *
 * <p>Scenario:
 * <ol>
 *   <li>Thread 1 acquires the lock on "account:42" and holds it for 1 second.</li>
 *   <li>Thread 2 tries to acquire the same lock. It retries 3 times (300ms total)
 *       before giving up or succeeding after Thread 1 releases.</li>
 *   <li>We log the timeline to show the serialization.</li>
 * </ol>
 *
 * <p>Redisson comparison: Redisson (https://github.com/redisson/redisson) is the
 * production choice for distributed locks in Java. Its key advantage over our
 * implementation: the watch-dog thread automatically renews the lock every
 * {@code leaseTime/3} — the caller never needs to renew manually. Our aspect
 * requires either a short TTL (risk of expiry during slow method) or manual
 * renewal (complexity). Redisson also implements fair locks (sorted set queue),
 * reentrant locks (per-thread counter), and a ReadWriteLock. Use Redisson in
 * production. Build this to understand what Redisson is doing underneath.
 */
@SpringBootApplication
public class LockDemoApplication implements CommandLineRunner {

    private static final Logger log = LoggerFactory.getLogger(LockDemoApplication.class);

    private final InventoryService inventoryService;

    public LockDemoApplication(InventoryService inventoryService) {
        this.inventoryService = inventoryService;
    }

    public static void main(String[] args) {
        SpringApplication.run(LockDemoApplication.class, args);
    }

    @Override
    public void run(String... args) throws Exception {
        log.info("=== Distributed Lock Demo ===");
        log.info("Two threads competing for the same account lock.");
        log.info("Thread 1 holds it for 1s. Thread 2 retries until it succeeds or gives up.");
        log.info("");

        CountDownLatch thread1Started = new CountDownLatch(1);
        AtomicReference<String> thread1Result = new AtomicReference<>();
        AtomicReference<String> thread2Result = new AtomicReference<>();

        ExecutorService pool = Executors.newFixedThreadPool(2);

        pool.submit(() -> {
            log.info("[Thread 1] Acquiring lock on account:42 with 3s TTL...");
            thread1Started.countDown();
            try {
                String result = inventoryService.debitAccount("42", 100);
                thread1Result.set(result);
                log.info("[Thread 1] Completed: {}", result);
            } catch (Exception e) {
                thread1Result.set("ERROR: " + e.getMessage());
                log.warn("[Thread 1] Failed: {}", e.getMessage());
            }
        });

        // Small delay so Thread 1 definitely acquires first.
        thread1Started.await();
        Thread.sleep(100);

        pool.submit(() -> {
            log.info("[Thread 2] Attempting to acquire lock on account:42 (Thread 1 holds it)...");
            try {
                String result = inventoryService.debitAccount("42", 50);
                thread2Result.set(result);
                log.info("[Thread 2] Completed: {}", result);
            } catch (Exception e) {
                thread2Result.set("BLOCKED: " + e.getMessage());
                log.info("[Thread 2] As expected — blocked: {}", e.getMessage());
            }
        });

        pool.shutdown();
        pool.awaitTermination(30, TimeUnit.SECONDS);

        log.info("");
        log.info("=== Results ===");
        log.info("Thread 1: {}", thread1Result.get());
        log.info("Thread 2: {}", thread2Result.get());
        log.info("");
        log.info("Note: If Thread 2 was blocked, increase retryAttempts in application.yml");
        log.info("      to allow it to wait longer for Thread 1 to release.");
    }
}

/**
 * Example service that uses @DistributedLock.
 * The aspect intercepts debitAccount and wraps it with acquire/release.
 */
@Service
class InventoryService {

    private static final Logger log = LoggerFactory.getLogger(InventoryService.class);

    @DistributedLock(resource = "account:42", ttlMs = 3_000)
    public String debitAccount(String accountId, int amount) throws InterruptedException {
        log.info("  [inside lock] processing debit of {} for account {}", amount, accountId);
        // Simulate a slow operation (e.g., database write).
        Thread.sleep(1_000);
        return String.format("debited %d from account %s at %s", amount, accountId, Instant.now());
    }
}
