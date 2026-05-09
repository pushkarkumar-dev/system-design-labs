package dev.pushkar.cron;

import net.javacrumbs.shedlock.core.LockProvider;
import net.javacrumbs.shedlock.provider.redis.spring.RedisLockProvider;
import net.javacrumbs.shedlock.spring.annotation.EnableSchedulerLock;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.data.redis.connection.RedisConnectionFactory;
import org.springframework.scheduling.annotation.EnableScheduling;

/**
 * Auto-configuration for distributed cron scheduling.
 *
 * EnableScheduling turns on the Spring task scheduler (the @Scheduled processor).
 * EnableSchedulerLock wraps each @SchedulerLock method with a distributed lock
 * check — only the node that acquires the lock actually executes the method body.
 *
 * defaultLockAtMostFor: the maximum time a lock can be held even if the holding
 * node crashes (equivalent to our Go LeaderLease.ExpiresAt).
 */
@Configuration
@EnableScheduling
@EnableSchedulerLock(defaultLockAtMostFor = "PT10M")
public class CronAutoConfiguration {

    /**
     * LockProvider backed by Redis. ShedLock uses SET NX PX (SET if Not eXists
     * with expiry in milliseconds) — the same primitive as our Go in-process CAS.
     *
     * In production the key looks like:
     *   shedlock:weeklyReport → {"lockUntil":"2026-05-08T09:10:00Z","lockedAt":"...","lockedBy":"pod-a"}
     */
    @Bean
    public LockProvider lockProvider(RedisConnectionFactory connectionFactory) {
        return new RedisLockProvider(connectionFactory);
    }
}
