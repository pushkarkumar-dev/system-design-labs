package dev.pushkar.rag;

import dev.langchain4j.service.AiServices;
import dev.langchain4j.service.UserMessage;
import dev.langchain4j.model.chat.ChatLanguageModel;
import dev.langchain4j.rag.content.retriever.ContentRetriever;
import org.springframework.stereotype.Service;

/**
 * Spring @Service that wires LangChain4j's {@code AiServices} with our Python retriever.
 *
 * <p>The key idea: {@code AiServices.builder()} creates a dynamic proxy that:
 * <ol>
 *   <li>Calls {@link ContentRetriever#retrieve} to get context chunks
 *   <li>Injects those chunks into the prompt automatically
 *   <li>Calls the {@link ChatLanguageModel} with the augmented prompt
 *   <li>Returns the answer as a String
 * </ol>
 *
 * <p>Our Python RAG backend is the retriever ({@link OurRetriever}).
 * The {@link ChatLanguageModel} is backed by any OpenAI-compatible endpoint
 * configured in {@code application.yml}.
 *
 * <p>From this class's perspective, the retriever is just an interface.
 * Swapping our Python service for a Pinecone retriever or a Weaviate retriever
 * requires changing only the {@link RagAutoConfiguration} — this class is unchanged.
 */
@Service
public class RagService {

    /**
     * LangChain4j AI interface — the framework generates the implementation.
     * The {@code @UserMessage} annotation becomes the prompt template.
     */
    interface Assistant {
        @UserMessage("Answer the following question based on the provided context: {{it}}")
        String answer(String question);
    }

    private final Assistant assistant;
    private final RagClient client;

    public RagService(ChatLanguageModel chatModel, ContentRetriever retriever, RagClient client) {
        this.client = client;
        this.assistant = AiServices.builder(Assistant.class)
                .chatLanguageModel(chatModel)
                .contentRetriever(retriever)
                .build();
    }

    /**
     * Answer a question using the Python RAG backend for retrieval and the
     * configured LLM for generation.
     */
    public String answer(String question) {
        return assistant.answer(question);
    }

    /**
     * Ingest documents directly through the Java client (bypasses LangChain4j).
     * Returns the number of chunks indexed.
     */
    public int ingest(java.util.List<String> docs) {
        return client.ingest(docs).chunksAdded();
    }
}
