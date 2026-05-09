package dev.pushkar.kafka;

import org.springframework.boot.context.properties.ConfigurationProperties;
import org.springframework.boot.context.properties.bind.DefaultValue;

import java.time.Duration;

/**
 * Configuration properties for the kafka-lite HTTP broker integration.
 *
 * <p>Bind via {@code @EnableConfigurationProperties(KafkaLiteProperties.class)} or
 * auto-configuration. All properties are prefixed with {@code kafka-lite}.
 *
 * <p>Example {@code application.yml}:
 * <pre>
 * kafka-lite:
 *   base-url: http://localhost:8080
 *   group-id: my-service
 *   poll-interval: 500ms
 *   auto-commit: false
 * </pre>
 */
@ConfigurationProperties("kafka-lite")
public record KafkaLiteProperties(
        /** Base URL of the kafka-lite Go broker. Default: http://localhost:8080 */
        @DefaultValue("http://localhost:8080") String baseUrl,

        /** Consumer group ID for this application instance. Default: default-group */
        @DefaultValue("default-group") String groupId,

        /**
         * How often the consumer polls the broker for new messages.
         * Equivalent to Kafka's {@code fetch.min.bytes} + poll loop interval.
         * Default: 500ms
         */
        @DefaultValue("500ms") Duration pollInterval,

        /**
         * Whether the consumer automatically commits the offset after each poll.
         * When false, the caller must call {@code KafkaLiteConsumer#commitOffset()} manually.
         *
         * <p>Equivalent to Kafka's {@code enable.auto.commit}.
         * Default: true
         */
        @DefaultValue("true") boolean autoCommit
) {}
