package dev.pushkar.quantizer;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Auto-configures the QuantizerClient bean with settings from application.yml.
 *
 * <p>Add to META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports
 * if packaging as a reusable library. For this demo, Spring Boot discovers it
 * via component scanning because it is in the application package.
 */
@AutoConfiguration
@EnableConfigurationProperties(QuantizerProperties.class)
public class QuantizerAutoConfiguration {

    @Bean
    public QuantizerClient quantizerClient(QuantizerProperties props) {
        return new QuantizerClient(props.baseUrl(), props.llamaServerUrl());
    }
}
