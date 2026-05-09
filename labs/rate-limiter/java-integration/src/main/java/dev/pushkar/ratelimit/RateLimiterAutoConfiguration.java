package dev.pushkar.ratelimit;

import org.springframework.boot.autoconfigure.condition.ConditionalOnProperty;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.web.servlet.config.annotation.InterceptorRegistry;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

/**
 * Auto-configuration that wires up the rate limiter components.
 *
 * <p>Beans created:
 * <ul>
 *   <li>{@link RateLimiterClient} — HTTP client for the Go service
 *   <li>{@link RateLimitInterceptor} — Spring MVC interceptor
 *   <li>{@link WebMvcConfigurer} — registers the interceptor with Spring MVC
 * </ul>
 *
 * <p>The interceptor is only registered when {@code rate-limiter.enabled=true}
 * (the default). Setting {@code rate-limiter.enabled=false} in
 * {@code application.yml} or an environment variable removes it entirely,
 * which is useful for local dev without a running Go service.
 */
@Configuration
@EnableConfigurationProperties(RateLimiterProperties.class)
public class RateLimiterAutoConfiguration {

    @Bean
    public RateLimiterClient rateLimiterClient(RateLimiterProperties props) {
        return new RateLimiterClient(props.serviceUrl(), props.defaultTier());
    }

    @Bean
    public RateLimitInterceptor rateLimitInterceptor(RateLimiterClient client,
                                                      RateLimiterProperties props) {
        return new RateLimitInterceptor(client, props);
    }

    /**
     * Register the interceptor with Spring MVC.
     *
     * <p>We apply it to all paths ({@code /**}) and exclude {@code /actuator/**}
     * so that health checks and metrics endpoints are never rate-limited.
     * Add more exclusions here as needed (e.g., {@code /public/**}).
     */
    @Bean
    @ConditionalOnProperty(name = "rate-limiter.enabled", havingValue = "true", matchIfMissing = true)
    public WebMvcConfigurer rateLimiterMvcConfigurer(RateLimitInterceptor interceptor) {
        return new WebMvcConfigurer() {
            @Override
            public void addInterceptors(InterceptorRegistry registry) {
                registry.addInterceptor(interceptor)
                        .addPathPatterns("/**")
                        .excludePathPatterns("/actuator/**");
            }
        };
    }
}
