package dev.pushkar.tokenizer;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.cache.annotation.EnableCaching;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration for the tokenizer service.
 *
 * Creates the TokenizerClient bean wired to the URL from application.yml.
 * The TokenizerService bean is created automatically by @Service + component
 * scan; it receives the client and properties via constructor injection.
 *
 * @EnableCaching activates the Spring cache abstraction, which backs
 * TokenizerService's Caffeine cache with proper cache stats and eviction.
 */
@Configuration
@EnableCaching
@EnableConfigurationProperties(TokenizerProperties.class)
public class TokenizerAutoConfiguration {

    /**
     * Creates the HTTP client that talks to the Python FastAPI server.
     *
     * The base URL comes from tokenizer.base-url in application.yml.
     * Default: http://localhost:8000
     */
    @Bean
    public TokenizerClient tokenizerClient(TokenizerProperties props) {
        return new TokenizerClient(props.baseUrl());
    }
}
