package dev.pushkar.docdb;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;
import java.util.Map;
import java.util.Optional;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link DocumentDbTemplate}.
 *
 * DocumentDbClient is mocked — these tests verify the template's behaviour
 * (POJO conversion, delegation, index call) without requiring a live server.
 */
@ExtendWith(MockitoExtension.class)
class DocumentDbTemplateTest {

    @Mock
    private DocumentDbClient client;

    private DocumentDbTemplate template;

    @BeforeEach
    void setUp() {
        template = new DocumentDbTemplate(client, new ObjectMapper());
    }

    /** insert() converts the POJO to a Map and returns the ID from the client. */
    @Test
    void insertReturnsDelegatedId() {
        when(client.insert(eq("users"), anyMap())).thenReturn("doc-uuid-123");

        record User(String email, int age) {}
        String id = template.insert("users", new User("alice@example.com", 30));

        assertThat(id).isEqualTo("doc-uuid-123");
        verify(client).insert(eq("users"), argThat(m -> "alice@example.com".equals(m.get("email"))));
    }

    /** find() returns deserialized POJOs for matching documents. */
    @Test
    void findReturnsMatchingDocuments() {
        var rawDocs = List.of(
                Map.<String, Object>of("email", "alice@example.com", "age", 30),
                Map.<String, Object>of("email", "bob@example.com",   "age", 25)
        );
        when(client.find(eq("users"), anyMap())).thenReturn(rawDocs);

        var results = template.find("users", Map.of("role", "user"), Map.class);

        assertThat(results).hasSize(2);
        assertThat(results.get(0)).containsEntry("email", "alice@example.com");
    }

    /** find() with an empty filter returns all documents (server returns full collection). */
    @Test
    void emptyFilterReturnsAllDocuments() {
        var rawDocs = List.of(
                Map.<String, Object>of("name", "Doc A"),
                Map.<String, Object>of("name", "Doc B"),
                Map.<String, Object>of("name", "Doc C")
        );
        when(client.find(eq("items"), eq(Map.of()))).thenReturn(rawDocs);

        var results = template.find("items", Map.of(), Map.class);

        assertThat(results).hasSize(3);
        verify(client).find("items", Map.of());
    }

    /** createIndex() delegates to client before find() — calling order matters. */
    @Test
    void createIndexDelegatesBeforeFind() {
        var rawDocs = List.of(Map.<String, Object>of("email", "alice@example.com"));
        when(client.find(eq("users"), anyMap())).thenReturn(rawDocs);

        // Simulate the typical pattern: create index, then find
        template.createIndex("users", "email");
        template.find("users", Map.of("email", "alice@example.com"), Map.class);

        var order = inOrder(client);
        order.verify(client).createIndex("users", "email");
        order.verify(client).find(eq("users"), anyMap());
    }

    /** findById() deserializes the raw map into the target POJO type. */
    @Test
    void findByIdDeserializesToPojo() {
        record User(String email, int age) {}

        when(client.get("users", "doc-123"))
                .thenReturn(Optional.of(Map.of("email", "alice@example.com", "age", 30)));

        Optional<User> result = template.findById("users", "doc-123", User.class);

        assertThat(result).isPresent();
        assertThat(result.get().email()).isEqualTo("alice@example.com");
        assertThat(result.get().age()).isEqualTo(30);
    }
}
