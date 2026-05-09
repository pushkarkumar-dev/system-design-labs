package dev.pushkar.docdb;

import com.fasterxml.jackson.databind.ObjectMapper;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.Map;
import java.util.Optional;

/**
 * Application-level document store template, analogous to Spring Data MongoDB's
 * {@code MongoTemplate}.
 *
 * <p>Differences from raw {@link DocumentDbClient}:
 * <ul>
 *   <li>Generic {@code <T>} insert/find: converts POJOs to/from {@code Map} automatically
 *       via Jackson's {@link ObjectMapper}.
 *   <li>Named collection per type convention: pass {@code User.class} to operate on
 *       the "users" collection by default, or supply a name explicitly.
 *   <li>Single {@link ObjectMapper} instance shared across the application —
 *       same configuration (date format, naming strategy) everywhere.
 * </ul>
 *
 * <p>This mirrors the design of {@code MongoTemplate} deliberately: if you later
 * swap the Rust server for a real MongoDB instance, the calling code does not change —
 * only the Spring wiring does.
 */
@Service
public class DocumentDbTemplate {

    private final DocumentDbClient client;
    private final ObjectMapper objectMapper;

    public DocumentDbTemplate(DocumentDbClient client, ObjectMapper objectMapper) {
        this.client = client;
        this.objectMapper = objectMapper;
    }

    /**
     * Insert a POJO into the named collection.
     * The object is serialized to a {@code Map} using Jackson, then stored.
     *
     * @param collection  collection name (e.g. "users")
     * @param document    any Jackson-serializable object
     * @return            the auto-generated document ID
     */
    public <T> String insert(String collection, T document) {
        @SuppressWarnings("unchecked")
        Map<String, Object> map = objectMapper.convertValue(document, Map.class);
        return client.insert(collection, map);
    }

    /**
     * Fetch a single document by ID and deserialize it into {@code type}.
     *
     * @param collection  collection name
     * @param id          document ID returned by {@link #insert}
     * @param type        target class
     * @return            deserialized document, or empty if not found
     */
    public <T> Optional<T> findById(String collection, String id, Class<T> type) {
        return client.get(collection, id)
                .map(map -> objectMapper.convertValue(map, type));
    }

    /**
     * Find all documents in a collection matching the equality filter.
     * Each matching document is deserialized into {@code type}.
     *
     * <p>If any filter key has a secondary index (created via
     * {@link #createIndex}), the server uses the index for an O(log N) lookup.
     * Otherwise it falls back to a full O(n) collection scan.
     *
     * @param collection  collection name
     * @param filter      equality conditions (empty = return all documents)
     * @param type        target class for deserialization
     * @return            list of matching documents, possibly empty
     */
    public <T> List<T> find(String collection, Map<String, Object> filter, Class<T> type) {
        return client.find(collection, filter).stream()
                .map(map -> objectMapper.convertValue(map, type))
                .toList();
    }

    /**
     * Create a secondary index on {@code field} within {@code collection}.
     *
     * <p>Secondary indexes speed up {@link #find} queries that filter on the
     * indexed field from O(n) to O(log N). The tradeoff: every subsequent
     * {@link #insert} must also update the index (write amplification).
     *
     * <p>Rule of thumb: only index fields with high cardinality (UUIDs, emails,
     * timestamps). Never index a boolean — you get at most a 2-way split.
     *
     * @param collection  collection name
     * @param field       field to index (must be a string, number, or boolean)
     */
    public void createIndex(String collection, String field) {
        client.createIndex(collection, field);
    }
}
