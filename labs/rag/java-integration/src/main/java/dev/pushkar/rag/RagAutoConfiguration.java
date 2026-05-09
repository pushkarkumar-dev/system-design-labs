package dev.pushkar.rag;

import dev.langchain4j.model.chat.ChatLanguageModel;
import dev.langchain4j.model.openai.OpenAiChatModel;
import dev.langchain4j.rag.content.retriever.ContentRetriever;
import org.springframework.boot.autoconfigure.AutoConfiguration;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;

/**
 * Spring Boot auto-configuration for the RAG integration.
 *
 * <p>Any Spring Boot application with this module on the classpath and
 * {@code rag.base-url} in {@code application.yml} gets a ready-to-inject
 * {@link RagService} bean with zero extra setup.
 *
 * <p>{@code @ConditionalOnMissingBean} means the app can override any bean
 * by declaring its own — standard Spring Boot customization contract.
 *
 * <p>Bean wiring:
 * <ol>
 *   <li>{@link RagClient}        — thin HTTP client for the Python server
 *   <li>{@link OurRetriever}     — LangChain4j ContentRetriever wrapping RagClient
 *   <li>{@link ChatLanguageModel}— OpenAI-compatible LLM for generation
 *   <li>{@link RagService}       — wires LangChain4j AiServices with the retriever
 * </ol>
 */
@AutoConfiguration
@EnableConfigurationProperties(RagProperties.class)
public class RagAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public RagClient ragClient(RagProperties props) {
        return new RagClient(props.baseUrl());
    }

    @Bean
    @ConditionalOnMissingBean
    public ContentRetriever ourRetriever(RagClient client) {
        return new OurRetriever(client, 3);
    }

    @Bean
    @ConditionalOnMissingBean
    public ChatLanguageModel chatLanguageModel(RagProperties props) {
        return OpenAiChatModel.builder()
                .baseUrl(props.openAiBaseUrl())
                .apiKey("not-needed")        // local servers typically don't check keys
                .modelName(props.model())
                .temperature(0.0)
                .build();
    }

    @Bean
    @ConditionalOnMissingBean
    public RagService ragService(ChatLanguageModel model, ContentRetriever retriever, RagClient client) {
        return new RagService(model, retriever, client);
    }
}
