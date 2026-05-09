package dev.pushkar.embedding;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Spring Boot auto-configuration for the embedding pipeline client.
 *
 * <p>Registers {@link EmbeddingClient} as a Spring bean configured from
 * {@link EmbeddingProperties}. The {@code baseUrl} defaults to
 * {@code http://localhost:8000} (our Python server) but can be overridden
 * in {@code application.yml}.
 *
 * <p>Note: Spring AI's {@code OpenAiEmbeddingClient} is auto-configured
 * separately by the {@code spring-ai-openai-spring-boot-starter} on the
 * classpath. Both beans are available — see {@link EmbeddingDemoApplication}
 * for how they're used side by side.
 */
@Configuration
@EnableConfigurationProperties(EmbeddingProperties.class)
public class EmbeddingAutoConfiguration {

    @Bean
    public EmbeddingClient embeddingClient(EmbeddingProperties props) {
        return new EmbeddingClient(props.getBaseUrl());
    }
}
