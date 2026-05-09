package dev.pushkar.promptopt;

import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Auto-configuration: registers {@link PromptOptClient} as a Spring bean
 * using properties from {@link PromptOptProperties}.
 */
@Configuration
@EnableConfigurationProperties(PromptOptProperties.class)
public class PromptOptAutoConfiguration {

    @Bean
    public PromptOptClient promptOptClient(PromptOptProperties props) {
        return new PromptOptClient(props.baseUrl());
    }
}
