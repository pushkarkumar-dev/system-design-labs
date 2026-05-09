package dev.pushkar.eval;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Auto-configuration for the LLM eval harness client.
 *
 * <p>Creates an {@link EvalClient} bean backed by {@link EvalProperties}.
 * Import this configuration or let Spring Boot's auto-configuration pick it up.
 */
@AutoConfiguration
@EnableConfigurationProperties(EvalProperties.class)
public class EvalAutoConfiguration {

    @Bean
    public EvalClient evalClient(EvalProperties props) {
        return new EvalClient(props);
    }
}
