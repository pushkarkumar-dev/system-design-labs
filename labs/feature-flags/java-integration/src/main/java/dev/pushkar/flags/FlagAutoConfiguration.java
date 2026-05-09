package dev.pushkar.flags;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.scheduling.annotation.EnableScheduling;

/**
 * Auto-configuration for the feature flag client.
 *
 * <p>Registers all beans required for @FeatureFlag-based feature gating:
 * <ul>
 *   <li>{@link FlagProperties} — bound from {@code feature-flags.*} config
 *   <li>{@link FlagClient}     — HTTP client for the Go flag server
 *   <li>{@link FlagCache}      — local cache with scheduled refresh and SSE
 *   <li>{@link FeatureFlagAspect} — AOP interceptor (picked up via @Component)
 * </ul>
 *
 * <p>In a full library setup, this class would be listed in
 * {@code META-INF/spring/org.springframework.boot.autoconfigure.AutoConfiguration.imports}
 * for zero-config usage. In this demo it is discovered via component scan.
 */
@AutoConfiguration
@EnableScheduling
@EnableConfigurationProperties(FlagProperties.class)
public class FlagAutoConfiguration {

    @Bean
    public FlagClient flagClient(FlagProperties props) {
        return new FlagClient(props.serviceUrl());
    }

    @Bean
    public FlagCache flagCache(FlagClient client, FlagProperties props, ObjectMapper mapper) {
        return new FlagCache(client, props, mapper);
    }
}
