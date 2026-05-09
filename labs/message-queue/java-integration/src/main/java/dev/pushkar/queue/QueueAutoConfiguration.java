package dev.pushkar.queue;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the message-queue HTTP client.
 *
 * <p>Any Spring Boot application with {@code queue.base-url} configured gets a
 * ready-to-inject {@link QueueClient} bean with zero extra setup.
 *
 * <p>Compare to the real AWS SDK's {@code SqsAutoConfiguration}, which wires
 * {@code SqsAsyncClient} and {@code SqsTemplate}. Our HTTP-bridge equivalent
 * shows the same structural patterns without the AWS dependency.
 */
@AutoConfiguration
@EnableConfigurationProperties(QueueProperties.class)
public class QueueAutoConfiguration {

    /**
     * Creates the {@link QueueClient} pointing at the configured Go server.
     * Override this bean to add custom interceptors (logging, auth headers).
     */
    @Bean
    @ConditionalOnMissingBean
    public QueueClient queueClient(QueueProperties props) {
        return new QueueClient(props.baseUrl());
    }
}
