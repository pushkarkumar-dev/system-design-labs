package dev.pushkar.crdt;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Spring auto-configuration for the CRDT demo integration.
 *
 * <p>Wires {@link CrdtProperties} and {@link CrdtClient} into the application context.
 * In a real Spring Boot auto-configuration this would live in
 * {@code META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports}.
 * Here it is a plain {@code @Configuration} for simplicity.
 */
@Configuration
@EnableConfigurationProperties(CrdtProperties.class)
public class CrdtAutoConfiguration {

    @Bean
    public CrdtClient crdtClient(CrdtProperties props) {
        return new CrdtClient(props.baseUrl());
    }
}
