package dev.pushkar.wal;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the WAL integration.
 *
 * <p>Any Spring Boot application that has this module on the classpath and
 * sets {@code wal.base-url} in {@code application.yml} gets a ready-to-inject
 * {@link WalService} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the app can override either bean
 * by declaring its own — standard Spring Boot customization contract.
 */
@AutoConfiguration
@EnableConfigurationProperties(WalProperties.class)
public class WalAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public WalClient walClient(WalProperties props) {
        return new WalClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public WalService walService(WalClient client, WalProperties props) {
        return new WalService(client, props);
    }
}
