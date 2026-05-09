package dev.pushkar.rag;

import dev.langchain4j.data.document.Metadata;
import dev.langchain4j.data.segment.TextSegment;
import dev.langchain4j.model.input.Prompt;
import dev.langchain4j.rag.content.Content;
import dev.langchain4j.rag.content.retriever.ContentRetriever;
import dev.langchain4j.rag.query.Query;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Collections;
import java.util.List;

/**
 * LangChain4j {@link ContentRetriever} that delegates to our Python RAG backend.
 *
 * <p>This is the plug-in point. LangChain4j's {@code AiServices} accepts any
 * {@link ContentRetriever} implementation — our Python service becomes a retriever
 * by wrapping it in this class. Any LangChain4j application can use our Python
 * RAG system as its knowledge source without knowing it's Python under the hood.
 *
 * <p>The contract: given a {@link Query}, return a {@link List} of {@link Content}
 * objects. Each {@link Content} wraps a {@link TextSegment} (the chunk text) plus
 * optional {@link Metadata}. LangChain4j injects these into the prompt context
 * automatically.
 *
 * <p>Implementation note: we call POST /query with top_k=3 (the retrieval half only —
 * the generation half happens inside LangChain4j via the configured ChatModel, not
 * inside the Python server). In practice this means the Python server's LLM call is
 * bypassed; we use it purely as a retriever.
 */
public class OurRetriever implements ContentRetriever {

    private static final Logger log = LoggerFactory.getLogger(OurRetriever.class);

    private final RagClient client;
    private final int topK;

    public OurRetriever(RagClient client, int topK) {
        this.client = client;
        this.topK   = topK;
    }

    public OurRetriever(RagClient client) {
        this(client, 3);
    }

    /**
     * Retrieve relevant content from our Python RAG backend for the given query.
     *
     * <p>Each source chunk from Python becomes a LangChain4j {@link Content}.
     * The chunk text is the segment text; source index and version are stored as metadata
     * so callers can trace which backend version produced each chunk.
     *
     * @param query LangChain4j query object (contains the user's question text)
     * @return list of Content objects, or empty list if the backend is unreachable
     */
    @Override
    public List<Content> retrieve(Query query) {
        String question = query.text();
        log.debug("Retrieving for question: {}", question);

        try {
            RagClient.RagResult result = client.query(question, topK);

            if (result.sources() == null || result.sources().isEmpty()) {
                log.debug("RAG backend returned empty sources for: {}", question);
                return Collections.emptyList();
            }

            return result.sources().stream()
                    .map(sourceText -> {
                        Metadata meta = Metadata.from(
                                "source", "python-rag-backend",
                                "version", result.version() != null ? result.version() : "unknown"
                        );
                        TextSegment segment = TextSegment.from(sourceText, meta);
                        return Content.from(segment);
                    })
                    .toList();

        } catch (Exception e) {
            log.warn("RAG backend unavailable: {}. Returning empty content list.", e.getMessage());
            return Collections.emptyList();
        }
    }
}
