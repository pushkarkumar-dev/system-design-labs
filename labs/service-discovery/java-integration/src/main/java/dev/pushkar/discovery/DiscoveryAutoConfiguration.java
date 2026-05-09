package dev.pushkar.discovery;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration that wires {@link DiscoveryClient} into the Spring context.
 *
 * <p>The client is configured from {@link DiscoveryProperties} which reads
 * {@code discovery.*} properties from {@code application.yml}.
 */
@Configuration
@EnableConfigurationProperties(DiscoveryProperties.class)
public class DiscoveryAutoConfiguration {

    @Bean
    public DiscoveryClient discoveryClient(DiscoveryProperties props) {
        return new DiscoveryClient(props.baseUrl());
    }
}
