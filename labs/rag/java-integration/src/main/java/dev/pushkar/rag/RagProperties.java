package dev.pushkar.rag;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Typed configuration for the RAG integration.
 *
 * <p>Bound from {@code application.yml} under the {@code rag} prefix.
 *
 * <pre>
 * rag:
 *   base-url: http://localhost:8000        # Python RAG server
 *   open-ai-base-url: http://localhost:8080 # OpenAI-compatible LLM
 *   model: local-model
 * </pre>
 */
@ConfigurationProperties(prefix = "rag")
public record RagProperties(
        /** Base URL of the Python RAG server (FastAPI). */
        String baseUrl,
        /** Base URL of the OpenAI-compatible LLM endpoint (for LangChain4j ChatModel). */
        String openAiBaseUrl,
        /** Model name passed to the OpenAI-compatible endpoint. */
        String model
) {
    public RagProperties {
        if (baseUrl == null)       baseUrl       = "http://localhost:8000";
        if (openAiBaseUrl == null) openAiBaseUrl = "http://localhost:8080";
        if (model == null)         model         = "local-model";
    }
}
