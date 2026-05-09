package dev.pushkar.httpserver;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Auto-configuration that registers {@link HttpServerClient} and
 * {@link HttpServerService} when they are not already present in the
 * application context.
 *
 * <p>The {@code @ConditionalOnMissingBean} guards allow tests and applications
 * to provide their own implementations (e.g. a mock) without conflicts.
 */
@AutoConfiguration
@EnableConfigurationProperties(HttpServerProperties.class)
public class HttpServerAutoConfiguration {

    /**
     * Creates a {@link HttpServerClient} configured with the base URL from
     * {@code http-lab.base-url} in application.yml.
     *
     * @param props resolved configuration properties
     * @return a ready-to-use client bean
     */
    @Bean
    @ConditionalOnMissingBean
    public HttpServerClient httpServerClient(HttpServerProperties props) {
        return new HttpServerClient(props.getBaseUrl());
    }

    /**
     * Creates a {@link HttpServerService} backed by the auto-configured client.
     *
     * @param client the WebClient wrapper
     * @return a ready-to-use service bean
     */
    @Bean
    @ConditionalOnMissingBean
    public HttpServerService httpServerService(HttpServerClient client) {
        return new HttpServerService(client);
    }
}
