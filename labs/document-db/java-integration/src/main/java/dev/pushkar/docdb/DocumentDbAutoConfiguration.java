package dev.pushkar.docdb;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Auto-configuration for the Document Database client beans.
 *
 * <p>Registers:
 * <ul>
 *   <li>{@link DocumentDbClient} — low-level HTTP client (≤60 lines)
 *   <li>{@link DocumentDbTemplate} — typed POJO template (MongoTemplate analogue)
 * </ul>
 *
 * <p>Both beans are conditional on {@code doc-db.base-url} being present.
 * In tests you can override them with a mock by declaring a bean of the same
 * type in the test context — Spring Boot auto-configuration backs off.
 */
@AutoConfiguration
@EnableConfigurationProperties(DocumentDbProperties.class)
public class DocumentDbAutoConfiguration {

    @Bean
    public DocumentDbClient documentDbClient(DocumentDbProperties props) {
        return new DocumentDbClient(props.baseUrl());
    }

    @Bean
    public DocumentDbTemplate documentDbTemplate(DocumentDbClient client,
                                                  ObjectMapper objectMapper) {
        return new DocumentDbTemplate(client, objectMapper);
    }
}
