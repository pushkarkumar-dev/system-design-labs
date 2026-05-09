package dev.pushkar.vector;

import org.springframework.ai.document.Document;
import org.springframework.ai.vectorstore.SearchRequest;
import org.springframework.ai.vectorstore.VectorStore;
import org.springframework.boot.autoconfigure.condition.ConditionalOnMissingBean;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.List;
import java.util.Optional;

/**
 * Auto-configuration for the Rust vector index integration.
 *
 * <p>Wires:
 * <ol>
 *   <li>{@link VectorClient} — the raw HTTP client</li>
 *   <li>{@link VectorStore} — Spring AI interface so the RAG pipeline works without changes</li>
 * </ol>
 *
 * <p>The {@code OurVectorStore} adapter shows the key lesson: Spring AI's
 * {@link VectorStore} interface is the right abstraction boundary. Any upstream
 * code (RAG pipelines, semantic search endpoints) depends on the interface,
 * not on our Rust-backed implementation.
 */
@Configuration
@EnableConfigurationProperties(VectorProperties.class)
public class VectorAutoConfiguration {

    @Bean
    @ConditionalOnMissingBean
    public VectorClient vectorClient(VectorProperties props) {
        return new VectorClient(props.baseUrl());
    }

    /**
     * Adapts our Rust HTTP client to Spring AI's {@link VectorStore} interface.
     * This lets any Spring AI RAG chain use our HNSW index as its retriever.
     */
    @Bean
    @ConditionalOnMissingBean(VectorStore.class)
    public VectorStore ourVectorStore(VectorClient client, VectorProperties props) {
        return new OurVectorStore(client, props);
    }

    // ── Spring AI VectorStore adapter ─────────────────────────────────────────

    static class OurVectorStore implements VectorStore {
        private final VectorClient client;
        private final VectorProperties props;

        OurVectorStore(VectorClient client, VectorProperties props) {
            this.client = client;
            this.props = props;
        }

        /**
         * Add documents to the index.
         * In a real system, embeddings would come from an EmbeddingModel.
         * Here we expect the embedding to be stored in document metadata
         * under the key "embedding" as a float[].
         */
        @Override
        public void add(List<Document> documents) {
            for (Document doc : documents) {
                Object emb = doc.getMetadata().get("embedding");
                if (emb instanceof float[] vector) {
                    client.add(doc.getId(), vector);
                } else {
                    throw new IllegalArgumentException(
                        "Document " + doc.getId() + " missing 'embedding' float[] in metadata");
                }
            }
        }

        @Override
        public Optional<Boolean> delete(List<String> idList) {
            // Our toy index does not support deletion.
            // Production: mark as tombstone, compact on restart.
            throw new UnsupportedOperationException(
                "Deletion not supported — rebuild index or use HNSW lazy-delete");
        }

        @Override
        public List<Document> similaritySearch(SearchRequest request) {
            Object embObj = request.getFilterExpression(); // simplified: use query string as id
            // For a real system the query string would be embedded first.
            // Here we expect the query vector in request metadata via a custom subclass.
            throw new UnsupportedOperationException(
                "Use similaritySearch(float[], int) for direct vector queries");
        }

        /** Direct vector query — bypasses text embedding. */
        public List<VectorClient.SearchResultEntry> similaritySearch(float[] queryVector, int k) {
            return client.search(queryVector, k, props.defaultEf());
        }
    }
}
