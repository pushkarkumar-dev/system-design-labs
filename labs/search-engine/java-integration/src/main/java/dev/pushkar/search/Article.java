package dev.pushkar.search;

/**
 * Domain object representing an indexed article.
 *
 * <p>Used by {@link SearchRepository} as the return type of
 * {@code findByContent(query)}. The {@code score} field is populated from
 * the BM25 ranking returned by the search engine; it is 0.0 for articles
 * that were fetched by ID rather than via a search query.</p>
 *
 * @param id    stable document identifier — the same value passed to
 *              {@link SearchClient#index(String, String)}
 * @param title human-readable title of the article
 * @param body  full article text (used for indexing)
 * @param score BM25 relevance score; higher = more relevant to the query
 */
public record Article(String id, String title, String body, double score) {

    /** Convenience factory for creating articles before indexing (score = 0). */
    public static Article of(String id, String title, String body) {
        return new Article(id, title, body, 0.0);
    }
}
