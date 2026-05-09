package dev.pushkar.search;

import java.util.List;

/**
 * Spring Data-style repository for full-text search over {@link Article} objects.
 *
 * <p>Following the Spring Data naming convention, {@code findByContent} signals
 * "find entities by searching their content field". The implementation delegates
 * to the Rust BM25 engine via {@link SearchClient}.</p>
 *
 * <p>Any Spring service can depend on this interface. Caching, pagination, and
 * auditing can be layered on top transparently — just as they would be with a
 * JPA or MongoDB repository.</p>
 */
public interface SearchRepository {

    /**
     * Find articles whose content matches the given free-text query.
     *
     * <p>Results are returned in BM25 relevance order — highest score first.
     * The number of results is bounded by {@link SearchProperties#defaultLimit()}.</p>
     *
     * @param query free-text query string
     * @return ranked list of matching articles; empty list when no results
     */
    List<Article> findByContent(String query);
}
