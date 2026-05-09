package dev.pushkar.saga;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the saga orchestrator client.
 *
 * <p>Any Spring Boot application that includes this module and sets
 * {@code saga.base-url} in {@code application.yml} gets a ready-to-inject
 * {@link SagaClient} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the application can override
 * the client by declaring its own bean — standard Spring Boot customization.
 */
@AutoConfiguration
@EnableConfigurationProperties(SagaProperties.class)
public class SagaAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public SagaClient sagaClient(SagaProperties props) {
        return new SagaClient(props.baseUrl());
    }
}
