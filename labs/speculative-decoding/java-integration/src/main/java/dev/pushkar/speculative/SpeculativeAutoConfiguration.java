package dev.pushkar.speculative;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Spring auto-configuration for the speculative decoding integration.
 *
 * <p>Creates and wires:
 * <ul>
 *   <li>{@link SpeculativeClient} — raw HTTP client for the Python FastAPI server
 * </ul>
 */
@Configuration
@EnableConfigurationProperties(SpeculativeProperties.class)
public class SpeculativeAutoConfiguration {

    @Bean
    public SpeculativeClient speculativeClient(SpeculativeProperties props) {
        return new SpeculativeClient(props);
    }
}
