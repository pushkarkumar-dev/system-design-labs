package dev.pushkar.kafka;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.web.client.RestClient;

/**
 * Spring Boot auto-configuration for the kafka-lite HTTP broker integration.
 *
 * <p>Any Spring Boot application that has this module on the classpath and
 * sets {@code kafka-lite.base-url} in {@code application.yml} gets ready-to-inject
 * {@link KafkaLiteProducer} and {@link KafkaLiteConsumer} beans with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the application can override any
 * bean by declaring its own — the standard Spring Boot customization contract.
 *
 * <p>Note: in production Kafka usage you would use {@code @EnableKafka} and
 * {@code KafkaAutoConfiguration} from {@code spring-kafka}. This auto-config
 * is the HTTP-bridge equivalent that shows the same structural patterns.
 */
@AutoConfiguration
@EnableConfigurationProperties(KafkaLiteProperties.class)
public class KafkaLiteAutoConfiguration {

    /**
     * Creates the shared {@link RestClient} for broker communication.
     * Override this bean to add custom interceptors (e.g., logging, auth headers).
     */
    @Bean
    @ConditionalOnMissingBean
    public RestClient kafkaLiteRestClient(KafkaLiteProperties props) {
        return RestClient.builder()
                .baseUrl(props.baseUrl())
                .build();
    }

    /**
     * Creates the {@link KafkaLiteProducer} pointing at the configured broker URL.
     * Equivalent to auto-configuring a {@code KafkaTemplate} bean.
     */
    @Bean
    @ConditionalOnMissingBean
    public KafkaLiteProducer kafkaLiteProducer(KafkaLiteProperties props) {
        return new KafkaLiteProducer(props.baseUrl());
    }
}
