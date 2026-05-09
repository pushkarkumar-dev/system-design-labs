package dev.pushkar.lock;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

/**
 * Marks a Spring-managed method as requiring a distributed lock.
 *
 * <p>Usage:
 * <pre>{@code
 * @DistributedLock(resource = "payment-#{#orderId}", ttlMs = 10_000)
 * public Receipt processPayment(String orderId, BigDecimal amount) { ... }
 * }</pre>
 *
 * <p>The {@link DistributedLockAspect} intercepts any method annotated with
 * this annotation, acquires the lock before the call, and releases it after
 * (even if the method throws).
 *
 * <p>The {@code resource} string supports a simple placeholder: any literal
 * value is used as-is. The aspect does not evaluate SpEL — keep resource
 * names static or compute them before calling the annotated method.
 */
@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.METHOD)
public @interface DistributedLock {

    /**
     * The resource name to lock. Must be non-empty.
     * All holders competing for the same resource must use the same name.
     */
    String resource();

    /**
     * Lock TTL in milliseconds. Defaults to 5,000ms.
     * The aspect will release the lock after the method returns, but if the
     * JVM crashes, the lock self-heals after this many milliseconds.
     */
    long ttlMs() default 5_000L;
}
