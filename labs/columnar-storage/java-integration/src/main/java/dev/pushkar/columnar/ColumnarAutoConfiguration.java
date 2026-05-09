package dev.pushkar.columnar;

import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the columnar storage integration.
 *
 * <p>Registers the {@link ColumnarClient} and {@link ParquetComparison} beans
 * using properties from {@link ColumnarProperties}.
 */
@AutoConfiguration
@EnableConfigurationProperties(ColumnarProperties.class)
public class ColumnarAutoConfiguration {

    @Bean
    public ColumnarClient columnarClient(ColumnarProperties props) {
        return new ColumnarClient(props.getBaseUrl());
    }

    @Bean
    public ParquetComparison parquetComparison() {
        return new ParquetComparison();
    }
}
