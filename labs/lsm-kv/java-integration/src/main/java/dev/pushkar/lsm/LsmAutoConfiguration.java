package dev.pushkar.lsm;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the LSM integration.
 *
 * <p>Any Spring Boot application with this module on the classpath and
 * {@code lsm.base-url} in {@code application.yml} gets a ready-to-inject
 * {@link LsmService} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the app can override either bean
 * by declaring its own — standard Spring Boot customization contract.
 *
 * <p>Typical override: provide a mock {@link LsmClient} in tests so the
 * service tests don't need a running Rust server.
 */
@AutoConfiguration
@EnableConfigurationProperties(LsmProperties.class)
public class LsmAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public LsmClient lsmClient(LsmProperties props) {
        return new LsmClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public LsmService lsmService(LsmClient client, LsmProperties props) {
        return new LsmService(client, props);
    }
}
