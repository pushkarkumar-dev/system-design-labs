package dev.pushkar.cicd;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration that wires the CI/CD engine client into the Spring context.
 *
 * This is the Spring Boot equivalent of the Go Executor struct:
 *   - Executor.Run(pipeline) → CicdClient.run(pipeline)
 *   - Per-step context timeout → cicd.default-timeout-seconds
 *   - Parallel stage steps → cicd.max-parallel-steps
 *
 * Spring's @Async + virtual threads (Java 21+) replace goroutines for
 * concurrent step execution inside a stage. The structural pattern is
 * identical: fan out, collect results, cancel on first failure.
 */
@Configuration
@EnableConfigurationProperties(CicdProperties.class)
public class CicdAutoConfiguration {

    /**
     * Expose the CicdClient as a bean so services can inject it with @Autowired.
     * The RestClient inside uses Spring's default connection pool (HttpClient 5),
     * which corresponds to Go's net/http Transport with keep-alives.
     */
    @Bean
    public CicdClient cicdClient(CicdProperties properties) {
        return new CicdClient(properties);
    }
}
