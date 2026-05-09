package dev.pushkar.lock;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.EnableAspectJAutoProxy;

/**
 * Auto-configuration for the distributed lock client and AOP aspect.
 *
 * <p>Registers:
 * <ul>
 *   <li>{@link LockClient} — HTTP client wired to the Go lock server URL from properties</li>
 *   <li>{@link DistributedLockAspect} — AOP aspect that intercepts {@code @DistributedLock} methods</li>
 * </ul>
 *
 * <p>Both beans are conditional on absence — applications can override either by
 * declaring their own beans.
 */
@AutoConfiguration
@EnableAspectJAutoProxy
@EnableConfigurationProperties(LockProperties.class)
public class LockAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public LockClient lockClient(LockProperties props) {
        return new LockClient(props.serviceUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public DistributedLockAspect distributedLockAspect(LockClient client, LockProperties props) {
        return new DistributedLockAspect(client, props);
    }
}
