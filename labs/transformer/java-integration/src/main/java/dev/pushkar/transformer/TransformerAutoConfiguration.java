package dev.pushkar.transformer;

import org.springframework.ai.chat.client.ChatClient;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the transformer integration.
 *
 * <p>Any Spring Boot application that has this module on the classpath and
 * sets {@code spring.ai.openai.base-url} in {@code application.yml} gets a
 * ready-to-inject {@link TransformerService} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the application can override
 * either bean by declaring its own — the standard Spring Boot customization
 * contract. If you want to add circuit-breaking (Resilience4j) or metrics
 * (Micrometer), extend {@link TransformerClient} and register your own bean.
 *
 * <p>Spring AI's {@link ChatClient} is already configured by
 * {@code spring-ai-openai-spring-boot-starter} using the base-url and api-key
 * from {@code spring.ai.openai.*}. We inject the auto-configured ChatClient
 * here and wrap it in our domain-specific client.
 */
@AutoConfiguration
@EnableConfigurationProperties(TransformerProperties.class)
public class TransformerAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public TransformerClient transformerClient(
            ChatClient.Builder chatClientBuilder,
            TransformerProperties props
    ) {
        // ChatClient.Builder is provided by Spring AI's auto-configuration.
        // It's already pointed at spring.ai.openai.base-url (our FastAPI server).
        ChatClient chatClient = chatClientBuilder.build();
        return new TransformerClient(chatClient, props);
    }

    @Bean
    @ConditionalOnMissingBean
    public TransformerService transformerService(
            TransformerClient client,
            TransformerProperties props,
            org.springframework.ai.openai.api.OpenAiApi openAiApi
    ) {
        return new TransformerService(client, props, openAiApi);
    }
}
