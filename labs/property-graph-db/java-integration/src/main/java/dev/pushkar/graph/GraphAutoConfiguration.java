package dev.pushkar.graph;

import org.neo4j.driver.AuthTokens;
import org.neo4j.driver.Driver;
import org.neo4j.driver.GraphDatabase;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Wires the Neo4j driver bean from GraphProperties.
 * Spring Data Neo4j auto-configures a driver from spring.neo4j.* properties,
 * but this explicit bean makes the configuration transparent for demo purposes.
 */
@Configuration
@EnableConfigurationProperties(GraphProperties.class)
public class GraphAutoConfiguration {

    @Bean
    public Driver neo4jDriver(GraphProperties props) {
        return GraphDatabase.driver(
            props.neo4jUri(),
            AuthTokens.basic(props.neo4jUsername(), props.neo4jPassword())
        );
    }
}
