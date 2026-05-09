package dev.pushkar.orchestrator;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for {@link OrchestratorClient}.
 *
 * <p>Registers the client bean and binds {@link OrchestratorProperties}
 * from {@code application.properties} / environment variables.
 */
@AutoConfiguration
@EnableConfigurationProperties(OrchestratorProperties.class)
public class OrchestratorAutoConfiguration {

    @Bean
    public OrchestratorClient orchestratorClient(OrchestratorProperties props) {
        return new OrchestratorClient(props);
    }
}
