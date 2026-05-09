package dev.pushkar.ratelimit;

import io.github.bucket4j.Bandwidth;
import io.github.bucket4j.Bucket;

import java.time.Duration;

/**
 * Side-by-side comparison of Bucket4j (in-process) vs our distributed service.
 *
 * <h3>When to use Bucket4j</h3>
 * <p>Bucket4j is a pure Java token bucket library. It runs in the same JVM
 * process as your application — no network calls, no external dependencies.
 * Throughput: millions of checks/sec (limited only by CAS operations on a
 * {@code LongAdder}, not network I/O).
 *
 * <p><b>Use Bucket4j when:</b>
 * <ul>
 *   <li>You have a single server instance (or accept that each instance has
 *       its own limit — i.e., N servers means N× the global limit)
 *   <li>You need sub-microsecond rate limiting (e.g., limiting DB queries
 *       inside a request)
 *   <li>You don't have a Redis/distributed store available
 * </ul>
 *
 * <h3>When to use our distributed service</h3>
 * <p><b>Use the distributed service when:</b>
 * <ul>
 *   <li>You run N server instances behind a load balancer and need a single
 *       global limit across all of them
 *   <li>The rate limit is for billing or abuse prevention (where accuracy matters)
 *   <li>You already have Redis in your stack (the marginal cost is one INCR)
 * </ul>
 *
 * <h3>Latency tradeoff</h3>
 * <p>Bucket4j: ~80ns per check (in-process CAS).
 * Distributed service: ~500µs per check (Redis INCR on loopback).
 *
 * <p>For a typical 10ms API response, 500µs is a 5% overhead. Acceptable for
 * most use cases. For sub-1ms APIs (e.g., latency-sensitive trading systems),
 * prefer Bucket4j with a periodic sync to Redis for approximate global counts.
 */
public class Bucket4jComparison {

    /**
     * Build a Bucket4j token bucket equivalent to our Go v0 implementation:
     * capacity=100, refillRate=100/60 per second (i.e., 100 per minute).
     *
     * <p>The API is nearly identical to our Go implementation. Both implement
     * the token bucket algorithm. The difference is deployment topology:
     * this bucket lives inside the JVM; our Go bucket lives in a separate
     * process (or is backed by Redis for distributed operation).
     */
    public static Bucket buildFreetierBucket() {
        return Bucket.builder()
                .addLimit(Bandwidth.builder()
                    .capacity(100)                       // burst ceiling: 100 requests
                    .refillGreedy(100, Duration.ofMinutes(1)) // refill rate: 100/min
                    .build())
                .build();
    }

    /**
     * Build a tiered bucket that matches our distributed limiter's tier structure.
     * In production, you'd keep one Bucket per user/key in a Caffeine cache.
     */
    public static Bucket buildTieredBucket(String tier) {
        long capacity = switch (tier) {
            case "premium" -> 10_000L;
            case "basic"   -> 1_000L;
            default        -> 100L;     // free tier
        };
        return Bucket.builder()
                .addLimit(Bandwidth.builder()
                    .capacity(capacity)
                    .refillGreedy(capacity, Duration.ofMinutes(1))
                    .build())
                .build();
    }

    /**
     * Demo: show that Bucket4j allows bursts (capacity=5, refill=5/min).
     * The first 5 calls succeed, the 6th fails — identical to our Go v0.
     */
    public static void main(String[] args) {
        Bucket bucket = Bucket.builder()
                .addLimit(Bandwidth.builder()
                    .capacity(5)
                    .refillGreedy(5, Duration.ofMinutes(1))
                    .build())
                .build();

        System.out.println("=== Bucket4j token bucket demo (capacity=5) ===");
        for (int i = 1; i <= 7; i++) {
            boolean allowed = bucket.tryConsume(1);
            System.out.printf("Request %d: %s%n", i, allowed ? "ALLOWED" : "DENIED (429)");
        }

        System.out.println();
        System.out.println("Compare with our Go distributed limiter:");
        System.out.println("  Bucket4j:    ~80ns/check — in-process, single server");
        System.out.println("  Distributed: ~500µs/check — Redis INCR, works across N servers");
        System.out.println();
        System.out.println("Rule of thumb:");
        System.out.println("  1 server  -> Bucket4j is faster AND accurate");
        System.out.println("  N servers -> Distributed is the only way to get global accuracy");
    }
}
