package dev.pushkar.lock;

import org.aspectj.lang.ProceedingJoinPoint;
import org.aspectj.lang.annotation.Around;
import org.aspectj.lang.annotation.Aspect;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;
import org.springframework.web.client.HttpClientErrorException;

import java.lang.management.ManagementFactory;
import java.util.concurrent.atomic.AtomicLong;

/**
 * AOP aspect that enforces distributed locking on methods annotated with
 * {@link DistributedLock}.
 *
 * <p>Advice lifecycle:
 * <ol>
 *   <li>Before method: acquire lock (retry up to {@code retryAttempts} times
 *       with {@code retryDelayMs} backoff between attempts)</li>
 *   <li>Call {@code joinPoint.proceed()} — the real method runs</li>
 *   <li>After method (finally): release lock regardless of success or exception</li>
 *   <li>If acquire never succeeds: throw {@link IllegalStateException}</li>
 *   <li>If storage rejects the fencing token (stale lock): wrap in {@link LockStolenException}</li>
 * </ol>
 *
 * <p>Why {@code @Around} and not {@code @Before} + {@code @After}?
 * {@code @Around} gives us the ability to wrap the method call in a try-finally,
 * ensuring the lock is always released. Separate {@code @Before} and {@code @AfterReturning}
 * cannot guarantee release on exception without {@code @AfterThrowing} too —
 * and coordinating token state across three advice methods is error-prone.
 */
@Aspect
@Component
public class DistributedLockAspect {

    private static final Logger log = LoggerFactory.getLogger(DistributedLockAspect.class);

    /**
     * Unique owner identifier: hostname:pid:counter.
     * Using a counter ensures each acquire attempt has a distinct owner string,
     * which is important when retrying after a partial failure.
     */
    private static final String OWNER_PREFIX = buildOwnerPrefix();
    private final AtomicLong acquireCounter = new AtomicLong();

    private final LockClient client;
    private final LockProperties props;

    public DistributedLockAspect(LockClient client, LockProperties props) {
        this.client = client;
        this.props = props;
    }

    /**
     * Intercepts methods annotated with {@code @DistributedLock}.
     *
     * <p>The pointcut expression targets any method (in any package) annotated
     * with {@code @DistributedLock}, regardless of visibility or class hierarchy.
     */
    @Around("@annotation(distributedLock)")
    public Object around(ProceedingJoinPoint joinPoint, DistributedLock distributedLock) throws Throwable {
        String resource = distributedLock.resource();
        long ttlMs = distributedLock.ttlMs() > 0 ? distributedLock.ttlMs() : props.defaultTtlMs();
        int maxAttempts = props.retryAttempts();
        long delayMs = props.retryDelayMs();

        String owner = OWNER_PREFIX + acquireCounter.incrementAndGet();
        long token = 0;
        boolean acquired = false;

        // ── Acquire phase ──────────────────────────────────────────────────
        for (int attempt = 1; attempt <= maxAttempts; attempt++) {
            LockClient.AcquireResult result = client.acquire(resource, owner, ttlMs);
            if (result != null && result.ok()) {
                token = result.token();
                acquired = true;
                log.debug("acquired lock: resource={} owner={} token={} attempt={}", resource, owner, token, attempt);
                break;
            }
            if (attempt < maxAttempts) {
                log.debug("lock busy, retrying: resource={} attempt={}/{}", resource, attempt, maxAttempts);
                Thread.sleep(delayMs);
            }
        }

        if (!acquired) {
            throw new IllegalStateException(
                String.format("Could not acquire lock on '%s' after %d attempts", resource, maxAttempts));
        }

        // ── Proceed + release phase ────────────────────────────────────────
        final long finalToken = token;
        try {
            return joinPoint.proceed();
        } catch (HttpClientErrorException.Conflict e) {
            // The storage server returned 409 — our fencing token was rejected.
            // This means another holder acquired the lock while we were in a
            // GC pause or network stall. Wrap and rethrow.
            throw new LockStolenException(resource, finalToken, e);
        } finally {
            try {
                client.release(resource, owner, finalToken);
                log.debug("released lock: resource={} owner={} token={}", resource, owner, finalToken);
            } catch (Exception releaseEx) {
                // Log but do not rethrow — the method already ran.
                // A failed release just means the lock expires on its own TTL.
                log.warn("failed to release lock: resource={} token={} — will expire on TTL", resource, finalToken, releaseEx);
            }
        }
    }

    private static String buildOwnerPrefix() {
        try {
            String name = ManagementFactory.getRuntimeMXBean().getName(); // "pid@hostname"
            return name + ":";
        } catch (Exception e) {
            return "unknown:";
        }
    }
}
