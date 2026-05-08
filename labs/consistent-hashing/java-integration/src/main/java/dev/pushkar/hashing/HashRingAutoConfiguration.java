package dev.pushkar.hashing;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the consistent hashing ring integration.
 *
 * <p>Any Spring Boot application that has this module on the classpath and
 * sets {@code ring.base-url} in {@code application.yml} gets a ready-to-inject
 * {@link HashRingRouter} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the application can override any
 * bean by declaring its own — the standard Spring Boot customization contract.
 * A common override: replace {@link HashRingClient} with a mock in integration tests.
 */
@AutoConfiguration
@EnableConfigurationProperties(HashRingProperties.class)
public class HashRingAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public HashRingClient hashRingClient(HashRingProperties props) {
        return new HashRingClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public HashRingRouter hashRingRouter(HashRingClient client, HashRingProperties props) {
        return new HashRingRouter(client, props);
    }
}
