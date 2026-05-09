package dev.pushkar.btree;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the B+Tree integration.
 *
 * <p>Any Spring Boot application with this module on the classpath and
 * {@code btree.base-url} in {@code application.yml} gets a ready-to-inject
 * {@link BTreeService} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} allows the application to override
 * either bean — for example, providing a mock {@link BTreeClient} in tests
 * so tests don't require a running Rust server.
 */
@AutoConfiguration
@EnableConfigurationProperties(BTreeProperties.class)
public class BTreeAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public BTreeClient btreeClient(BTreeProperties props) {
        return new BTreeClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public BTreeService btreeService(BTreeClient client, BTreeProperties props) {
        return new BTreeService(client, props);
    }
}
