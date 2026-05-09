package dev.pushkar.faas;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration for the FaaS client and Spring Cloud Function comparison.
 *
 * <p>Registers {@link FaasClient} and {@link SpringCloudFunctionComparison}
 * as Spring beans, bound to {@link FaasProperties} configuration.
 */
@Configuration
@EnableConfigurationProperties(FaasProperties.class)
public class FaasAutoConfiguration {

    @Bean
    public FaasClient faasClient(FaasProperties props) {
        return new FaasClient(props);
    }

    @Bean
    public SpringCloudFunctionComparison springCloudFunctionComparison() {
        return new SpringCloudFunctionComparison();
    }
}
