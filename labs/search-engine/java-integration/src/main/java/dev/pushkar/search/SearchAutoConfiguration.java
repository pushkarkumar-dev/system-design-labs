package dev.pushkar.search;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the search engine integration.
 *
 * <p>Registers {@link SearchClient} and {@link SearchRepositoryImpl} as beans
 * only when the application has not already defined its own versions
 * (via {@link ConditionalOnMissingBean}). This follows the standard
 * Spring Boot library pattern: provide sensible defaults that yield to
 * application-level overrides.</p>
 *
 * <p>To override the client URL programmatically, declare your own
 * {@code SearchClient} bean in an {@code @Configuration} class:</p>
 * <pre>
 * {@code @Bean
 * SearchClient searchClient() {
 *     return new SearchClient("http://search.internal:9000");
 * }}
 * </pre>
 */
@AutoConfiguration
@EnableConfigurationProperties(SearchProperties.class)
public class SearchAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public SearchClient searchClient(SearchProperties props) {
        return new SearchClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public SearchRepositoryImpl searchRepository(SearchClient client,
                                                  SearchProperties props) {
        return new SearchRepositoryImpl(client, props);
    }
}
