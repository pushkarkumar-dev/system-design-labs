package dev.pushkar.search;

import org.springframework.stereotype.Repository;

import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * {@link SearchRepository} implementation backed by {@link SearchClient}.
 *
 * <p>Keeps an in-process article store (title + body) so that search results
 * can be enriched with full article metadata. In production this would be a
 * database; here it is a {@link ConcurrentHashMap} to keep the demo self-contained.</p>
 *
 * <p>The class is annotated {@code @Repository} so Spring detects it during
 * component scanning and treats infrastructure exceptions uniformly
 * (Spring wraps them in {@code DataAccessException}).</p>
 */
@Repository
public class SearchRepositoryImpl implements SearchRepository {

    private final SearchClient client;
    private final SearchProperties props;

    /** In-process store: docId → Article (without score). */
    private final Map<String, Article> store = new ConcurrentHashMap<>();

    public SearchRepositoryImpl(SearchClient client, SearchProperties props) {
        this.client = client;
        this.props = props;
    }

    /**
     * Index an article so it becomes searchable via {@link #findByContent}.
     *
     * @param article article to index; {@code article.score()} is ignored
     */
    public void index(Article article) {
        store.put(article.id(), article);
        client.index(article.id(), article.title() + " " + article.body());
    }

    /**
     * Remove an article from both the local store and the remote index.
     *
     * @param docId document identifier to remove
     */
    public void delete(String docId) {
        store.remove(docId);
        client.deleteDoc(docId);
    }

    /**
     * {@inheritDoc}
     *
     * <p>Calls {@link SearchClient#search} with {@link SearchProperties#defaultLimit()},
     * then enriches each result with the stored article metadata (title, body).
     * Results that cannot be found in the local store (e.g. race with a concurrent
     * delete) are silently dropped.</p>
     */
    @Override
    public List<Article> findByContent(String query) {
        if (query == null || query.isBlank()) {
            return List.of();
        }

        List<SearchResult> ranked = client.search(query, props.defaultLimit());

        return ranked.stream()
                .filter(r -> store.containsKey(r.docId()))
                .map(r -> {
                    Article base = store.get(r.docId());
                    // Return a new record with the BM25 score populated
                    return new Article(base.id(), base.title(), base.body(), r.score());
                })
                .toList();
    }
}
