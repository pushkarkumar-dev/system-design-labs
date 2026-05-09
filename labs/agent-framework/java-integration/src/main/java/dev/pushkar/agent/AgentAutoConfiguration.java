package dev.pushkar.agent;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration for the agent framework client.
 *
 * <p>Registers {@link AgentClient} as a Spring bean, configured from
 * {@link AgentProperties} (i.e. from {@code agent.base-url} in application.yml).
 */
@Configuration
@EnableConfigurationProperties(AgentProperties.class)
public class AgentAutoConfiguration {

    @Bean
    public AgentClient agentClient(AgentProperties props) {
        return new AgentClient(props.getBaseUrl());
    }
}
