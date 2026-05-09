package dev.pushkar.container;

import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration for the container runtime lab demo beans.
 *
 * <p>{@code @ConditionalOnMissingBean} ensures that test configurations can
 * override any bean without the auto-configuration replacing it.
 */
@Configuration
@EnableConfigurationProperties(ContainerProperties.class)
public class ContainerAutoConfiguration {

    /**
     * Registers the {@link ContainerClient} bean.
     *
     * <p>In production this would be something like a Docker API client or
     * a wrapper around Testcontainers' GenericContainer. Here it documents
     * the relationship between our Go runtime and Testcontainers.
     */
    @Bean
    @ConditionalOnMissingBean
    public ContainerClient containerClient(ContainerProperties props) {
        return new ContainerClient(props);
    }
}
