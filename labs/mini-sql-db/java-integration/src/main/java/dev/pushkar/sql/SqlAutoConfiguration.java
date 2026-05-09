package dev.pushkar.sql;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Auto-configuration for the mini-sql-db client.
 *
 * <p>Registers a {@link SqlClient} bean using {@link SqlProperties}.
 * The {@code @ConditionalOnMissingBean} annotation allows applications
 * to override the client by defining their own {@code SqlClient} bean.
 */
@AutoConfiguration
@EnableConfigurationProperties(SqlProperties.class)
public class SqlAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public SqlClient sqlClient(SqlProperties props) {
        return new SqlClient(props.baseUrl());
    }
}
