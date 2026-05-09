package dev.pushkar.rag;

import dev.langchain4j.rag.content.Content;
import dev.langchain4j.rag.query.Query;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.web.client.ResourceAccessException;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyInt;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

/**
 * Unit tests for {@link OurRetriever}.
 *
 * <p>All tests mock {@link RagClient} — no network or Python server required.
 * This follows the "test the boundary" pattern: we test that OurRetriever correctly
 * maps RagClient responses to LangChain4j Content objects, handles errors gracefully,
 * and preserves metadata.
 */
@ExtendWith(MockitoExtension.class)
class OurRetrieverTest {

    @Mock
    private RagClient mockClient;

    private OurRetriever retriever;

    @BeforeEach
    void setUp() {
        retriever = new OurRetriever(mockClient, 3);
    }

    // ── Test 1: successful retrieval returns correct Content list ─────────────

    @Test
    void retrieve_returnsContentList_whenBackendReturnsSourceChunks() {
        // Arrange
        var sources = List.of(
                "WAL ensures durability by writing to a log first.",
                "PostgreSQL and MySQL InnoDB both use WAL."
        );
        var ragResult = new RagClient.RagResult("WAL answer", sources, "v1-hybrid");
        when(mockClient.query("What is WAL?", 3)).thenReturn(ragResult);

        // Act
        Query query = Query.from("What is WAL?");
        List<Content> contents = retriever.retrieve(query);

        // Assert
        assertThat(contents).hasSize(2);
        assertThat(contents.get(0).textSegment().text())
                .isEqualTo("WAL ensures durability by writing to a log first.");
        assertThat(contents.get(1).textSegment().text())
                .isEqualTo("PostgreSQL and MySQL InnoDB both use WAL.");
        verify(mockClient).query("What is WAL?", 3);
    }

    // ── Test 2: empty sources returns empty list ──────────────────────────────

    @Test
    void retrieve_returnsEmptyList_whenBackendReturnsEmptySources() {
        // Arrange
        var ragResult = new RagClient.RagResult("No relevant chunks found.", List.of(), "v0-naive");
        when(mockClient.query(anyString(), anyInt())).thenReturn(ragResult);

        // Act
        List<Content> contents = retriever.retrieve(Query.from("obscure question"));

        // Assert
        assertThat(contents).isEmpty();
    }

    // ── Test 3: query timeout / network error returns empty list gracefully ───

    @Test
    void retrieve_returnsEmptyList_whenBackendThrowsException() {
        // Arrange — simulate a connection timeout
        when(mockClient.query(anyString(), anyInt()))
                .thenThrow(new ResourceAccessException("Connection refused"));

        // Act — should NOT propagate the exception; logs a warning and returns empty
        List<Content> contents = retriever.retrieve(Query.from("any question"));

        // Assert
        assertThat(contents).isEmpty();
    }

    // ── Test 4: source metadata is preserved in Content objects ──────────────

    @Test
    void retrieve_preservesSourceMetadata_inTextSegment() {
        // Arrange
        var sources = List.of("Consistent hashing uses a hash ring.");
        var ragResult = new RagClient.RagResult("answer", sources, "v1-hybrid");
        when(mockClient.query(anyString(), anyInt())).thenReturn(ragResult);

        // Act
        List<Content> contents = retriever.retrieve(Query.from("consistent hashing"));

        // Assert — metadata should record the backend source and version
        assertThat(contents).hasSize(1);
        var meta = contents.get(0).textSegment().metadata();
        assertThat(meta.getString("source")).isEqualTo("python-rag-backend");
        assertThat(meta.getString("version")).isEqualTo("v1-hybrid");
    }

    // ── Test 5: LangChain4j integration — retriever called by AiServices ─────

    @Test
    void retrieve_delegatesToRagClient_withCorrectTopK() {
        // Arrange
        OurRetriever retrieverWithTopK5 = new OurRetriever(mockClient, 5);
        var ragResult = new RagClient.RagResult("answer", List.of("chunk1"), "v2-rerank");
        when(mockClient.query("test question", 5)).thenReturn(ragResult);

        // Act
        retrieverWithTopK5.retrieve(Query.from("test question"));

        // Assert — verify top_k=5 was passed to the client
        verify(mockClient).query("test question", 5);
    }
}
