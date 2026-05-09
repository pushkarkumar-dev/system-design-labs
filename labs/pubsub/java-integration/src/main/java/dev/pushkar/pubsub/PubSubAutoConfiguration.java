package dev.pushkar.pubsub;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the PubSubClient.
 *
 * <p>Registers a singleton {@link PubSubClient} bean pointing at
 * {@code pubsub.broker-url} (default: {@code http://localhost:8080}).
 */
@AutoConfiguration
@EnableConfigurationProperties(PubSubProperties.class)
public class PubSubAutoConfiguration {

    @Bean
    public PubSubClient pubSubClient(PubSubProperties props) {
        return new PubSubClient(props.getBrokerUrl());
    }
}
