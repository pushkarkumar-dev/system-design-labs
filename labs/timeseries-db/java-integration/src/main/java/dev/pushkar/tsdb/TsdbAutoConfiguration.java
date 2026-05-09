package dev.pushkar.tsdb;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the TSDB Micrometer integration.
 *
 * <p>Any Spring Boot application that has this module on the classpath and
 * sets {@code tsdb.base-url} in {@code application.yml} gets:
 * <ul>
 *   <li>A {@link TsdbClient} bean wired to the configured URL</li>
 *   <li>A {@link TsdbMicrometerRegistry} bean that automatically pushes all
 *       registered meters to the TSDB on every {@code push-interval} tick</li>
 * </ul>
 *
 * <p>{@code @ConditionalOnMissingBean} lets the application override either
 * bean by declaring its own — standard Spring Boot customization contract.
 */
@AutoConfiguration
@EnableConfigurationProperties(TsdbProperties.class)
public class TsdbAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public TsdbClient tsdbClient(TsdbProperties props) {
        return new TsdbClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public TsdbMicrometerRegistry tsdbMicrometerRegistry(
            TsdbClient client, TsdbProperties props) {
        return new TsdbMicrometerRegistry(client, props);
    }
}
