package dev.pushkar.stream;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Auto-configuration for the stream processor client.
 * Registers {@link StreamProcessorClient} as a Spring bean using
 * the {@code stream-processor.base-url} property.
 */
@AutoConfiguration
@EnableConfigurationProperties(StreamProperties.class)
public class StreamAutoConfiguration {

    @Bean
    public StreamProcessorClient streamProcessorClient(StreamProperties props) {
        return new StreamProcessorClient(props.getBaseUrl());
    }

    @Bean
    public KafkaStreamsComparison kafkaStreamsComparison() {
        return new KafkaStreamsComparison();
    }
}
