package dev.pushkar.sql;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Rust mini-sql-db server.
 *
 * <p>Sends SQL strings to {@code POST /query} and parses the JSON response.
 * The server returns: {@code {"columns": [...], "rows": [{col: val, ...}], "message": "..."}}
 *
 * <p>Design: ≤ 60 lines, no retry logic (that belongs in a service layer),
 * uses Spring Framework 6.1's {@link RestClient} (not deprecated RestTemplate).
 *
 * <p>Usage:
 * <pre>
 *   SqlClient client = new SqlClient("http://localhost:7070");
 *   QueryResult r = client.query("SELECT * FROM users WHERE id = 1");
 *   r.rows().forEach(row -> System.out.println(row.get("name")));
 * </pre>
 */
public class SqlClient {

    private final RestClient http;

    public SqlClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Execute a SQL statement and return the result.
     *
     * @param sql any SQL the Rust engine supports (SELECT, INSERT, CREATE TABLE, etc.)
     * @return QueryResult with column names, rows as String maps, and a status message
     * @throws SqlException on non-2xx responses or network errors
     */
    public QueryResult query(String sql) {
        try {
            var result = http.post()
                    .uri("/query")
                    .contentType(MediaType.TEXT_PLAIN)
                    .body(sql)
                    .retrieve()
                    .body(QueryResult.class);
            return result != null ? result : new QueryResult(List.of(), List.of(), "null response");
        } catch (RestClientException e) {
            throw new SqlException("Query failed: " + e.getMessage(), e);
        }
    }

    /** Check whether the Rust server is reachable. */
    public boolean isHealthy() {
        try {
            http.get().uri("/health").retrieve().toBodilessEntity();
            return true;
        } catch (RestClientException e) {
            return false;
        }
    }

    // ── Response types ───────────────────────────────────────────────────────

    /**
     * Result of a SQL query.
     *
     * @param columns ordered list of column names in the result set
     * @param rows    each row is a map from column name to its string representation
     * @param message human-readable status ("2 row(s)", "CREATE TABLE users", etc.)
     */
    public record QueryResult(
        List<String> columns,
        List<Map<String, String>> rows,
        String message
    ) {
        /** Convenience: get a typed value from a row by column name. */
        public String get(Map<String, String> row, String column) {
            return row.getOrDefault(column, null);
        }
    }

    public static class SqlException extends RuntimeException {
        public SqlException(String msg, Throwable cause) { super(msg, cause); }
    }
}
