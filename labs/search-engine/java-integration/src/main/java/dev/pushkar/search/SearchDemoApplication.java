package dev.pushkar.search;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.util.List;

/**
 * Runnable demo that indexes five database-related articles, runs a
 * full-text search, and prints the BM25-ranked results.
 *
 * <p>Prerequisites: the Rust search server must be running on port 8080.
 * From {@code labs/search-engine}:
 * <pre>
 *   cargo run --bin search-server -- --port 8080
 * </pre>
 * Then start this application:
 * <pre>
 *   cd java-integration && mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class SearchDemoApplication implements CommandLineRunner {

    private final SearchRepositoryImpl repo;

    public SearchDemoApplication(SearchRepositoryImpl repo) {
        this.repo = repo;
    }

    public static void main(String[] args) {
        SpringApplication.run(SearchDemoApplication.class, args);
    }

    @Override
    public void run(String... args) {
        System.out.println("\n=== Search Engine Spring Integration Demo ===\n");

        // Index five articles on database topics
        List<Article> articles = List.of(
            Article.of("art-1",
                "Introduction to Database Indexes",
                "A database index is a data structure that improves the speed of data retrieval. " +
                "B-tree indexes are the default in PostgreSQL and MySQL. Index scans avoid full table scans. " +
                "Composite indexes cover multiple columns. Index performance depends on cardinality and selectivity."),

            Article.of("art-2",
                "Write-Ahead Logging and Durability",
                "Write-ahead logging (WAL) ensures durability by writing changes to a log before " +
                "applying them to the main data structure. WAL is used by PostgreSQL, RocksDB, and SQLite. " +
                "Recovery replays the log to reconstruct committed state after a crash."),

            Article.of("art-3",
                "BM25 Ranking in Search Engines",
                "BM25 is the default ranking function in Elasticsearch and Apache Solr. It extends " +
                "TF-IDF with saturation (k1=1.5) and length normalization (b=0.75). " +
                "BM25 penalizes long documents and term stuffing. Performance on keyword queries " +
                "remains unbeaten even against neural retrieval models with no training data."),

            Article.of("art-4",
                "LSM-Tree Storage Engines",
                "Log-structured merge trees buffer writes in memory (memtable) then flush immutable " +
                "SSTables to disk. Compaction merges SSTables to reclaim space and maintain read " +
                "performance. RocksDB and Cassandra use LSM trees. Write amplification is the key " +
                "tuning metric alongside read performance and space amplification."),

            Article.of("art-5",
                "Inverted Index Construction",
                "An inverted index maps each term to the list of documents containing it (posting list). " +
                "Posting lists are stored sorted by document ID to enable O(n+m) AND intersection. " +
                "Delta encoding plus varint compression reduces posting list size by 7:1. " +
                "Lucene uses immutable index segments to enable concurrent reads without locking.")
        );

        System.out.println("Indexing " + articles.size() + " articles...");
        for (Article a : articles) {
            repo.index(a);
            System.out.println("  indexed: " + a.id() + " — " + a.title());
        }

        System.out.println("\nSearching for: \"database index performance\"\n");
        List<Article> results = repo.findByContent("database index performance");

        if (results.isEmpty()) {
            System.out.println("No results (is the Rust server running on port 8080?)");
        } else {
            System.out.printf("%-8s  %-6s  %s%n", "DocId", "Score", "Title");
            System.out.println("-".repeat(60));
            for (Article r : results) {
                System.out.printf("%-8s  %5.3f  %s%n", r.id(), r.score(), r.title());
            }
        }

        System.out.println("\nDone.");
    }
}
