package dev.pushkar.search;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link SearchRepositoryImpl}.
 *
 * <p>The Rust search server is mocked via Mockito so these tests run offline.
 * Five scenarios are covered:
 * <ol>
 *   <li>Index + search roundtrip — indexed articles appear in results</li>
 *   <li>Results are ordered by BM25 score descending</li>
 *   <li>Empty query returns empty list without calling the server</li>
 *   <li>Deleted article is absent from subsequent results</li>
 *   <li>Limit is respected — only {@code defaultLimit} results returned</li>
 * </ol>
 */
@ExtendWith(MockitoExtension.class)
class SearchRepositoryTest {

    @Mock
    private SearchClient client;

    private SearchRepositoryImpl repo;

    private static final SearchProperties PROPS =
            new SearchProperties("http://localhost:8080", 10);

    @BeforeEach
    void setUp() {
        repo = new SearchRepositoryImpl(client, PROPS);
    }

    // ── Test 1: index + search roundtrip ─────────────────────────────────────

    @Test
    void indexedArticleAppearsInSearchResults() {
        Article article = Article.of("doc-1", "Database Indexes", "B-tree index performance");
        when(client.search(eq("index"), eq(10)))
                .thenReturn(List.of(new SearchResult("doc-1", 1.5)));

        repo.index(article);
        List<Article> results = repo.findByContent("index");

        assertThat(results).hasSize(1);
        assertThat(results.get(0).id()).isEqualTo("doc-1");
        assertThat(results.get(0).title()).isEqualTo("Database Indexes");
        verify(client).index(eq("doc-1"), contains("Database Indexes"));
    }

    // ── Test 2: result ordering by BM25 score ────────────────────────────────

    @Test
    void resultsAreOrderedByScoreDescending() {
        Article a1 = Article.of("doc-1", "Index Basics", "database index scan");
        Article a2 = Article.of("doc-2", "Index Deep Dive", "database index performance tuning");
        Article a3 = Article.of("doc-3", "WAL Internals", "write ahead log durability");

        // Engine returns results already sorted by BM25 score
        when(client.search(eq("database index"), eq(10)))
                .thenReturn(List.of(
                        new SearchResult("doc-2", 2.4),
                        new SearchResult("doc-1", 1.8),
                        new SearchResult("doc-3", 0.6)
                ));

        repo.index(a1);
        repo.index(a2);
        repo.index(a3);

        List<Article> results = repo.findByContent("database index");

        assertThat(results).hasSize(3);
        assertThat(results.get(0).score()).isGreaterThan(results.get(1).score());
        assertThat(results.get(1).score()).isGreaterThan(results.get(2).score());
        assertThat(results.get(0).id()).isEqualTo("doc-2"); // highest BM25 score
    }

    // ── Test 3: empty query returns empty list ────────────────────────────────

    @Test
    void emptyQueryReturnsEmptyListWithoutCallingServer() {
        List<Article> results = repo.findByContent("");

        assertThat(results).isEmpty();
        verify(client, never()).search(any(), anyInt());
    }

    // ── Test 4: delete removes article from results ───────────────────────────

    @Test
    void deleteRemovesArticleFromSubsequentResults() {
        Article article = Article.of("doc-1", "Inverted Index", "posting list compression");

        // After delete, client returns the doc in results but it should be filtered out
        when(client.search(eq("posting"), eq(10)))
                .thenReturn(List.of(new SearchResult("doc-1", 1.2)));

        repo.index(article);
        repo.delete("doc-1");

        List<Article> results = repo.findByContent("posting");

        // Local store no longer has doc-1, so it is dropped from enrichment
        assertThat(results).isEmpty();
        verify(client).deleteDoc(eq("doc-1"));
    }

    // ── Test 5: limit is respected ────────────────────────────────────────────

    @Test
    void limitIsRespectedWhenSearchingWithSmallDefaultLimit() {
        SearchProperties limitedProps = new SearchProperties("http://localhost:8080", 2);
        SearchRepositoryImpl limitedRepo = new SearchRepositoryImpl(client, limitedProps);

        for (int i = 1; i <= 5; i++) {
            limitedRepo.index(Article.of("doc-" + i, "Title " + i, "content about databases"));
        }

        when(client.search(eq("databases"), eq(2)))
                .thenReturn(List.of(
                        new SearchResult("doc-1", 1.9),
                        new SearchResult("doc-2", 1.7)
                ));

        List<Article> results = limitedRepo.findByContent("databases");

        assertThat(results).hasSize(2);
        verify(client).search(eq("databases"), eq(2));  // limit=2 passed to server
    }
}
