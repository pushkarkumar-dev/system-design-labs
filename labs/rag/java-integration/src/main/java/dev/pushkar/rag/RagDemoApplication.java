package dev.pushkar.rag;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;

/**
 * Demo Spring Boot application that exercises the RAG integration.
 *
 * <p>Start the Python RAG server first (from labs/rag/):
 * <pre>
 *   RAG_VERSION=v1 uvicorn src.server:app --port 8000
 * </pre>
 *
 * <p>Then run this demo:
 * <pre>
 *   cd labs/rag/java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class RagDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(RagDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(RagService rag, RagClient client) {
        return args -> {
            System.out.println("=== RAG Spring Integration Demo ===\n");

            // 1. Check health
            var health = client.health();
            System.out.printf("RAG backend health: %s (chunks indexed: %d, version: %s)%n%n",
                    health.status(), health.totalChunks(), health.version());

            // 2. Ingest 5 documents
            var docs = List.of(
                    "Write-ahead logging (WAL) ensures durability by writing changes to a log before applying them to the primary data structure. PostgreSQL, MySQL InnoDB, and RocksDB all use WAL as their primary durability mechanism.",
                    "Consistent hashing distributes data across nodes using a hash ring. When a node is added or removed, only the keys between the new and old positions need to move. Cassandra and DynamoDB use consistent hashing for partitioning.",
                    "The LSM-tree (Log-Structured Merge-tree) optimizes write-heavy workloads. Writes go first to an in-memory memtable. When the memtable fills, it flushes to an immutable SSTable on disk. RocksDB and LevelDB are LSM-tree implementations.",
                    "Reciprocal Rank Fusion (RRF) combines multiple ranked lists by scoring each document as the sum of 1/(k + rank_i) across all systems, where k=60 prevents top-ranked documents from dominating.",
                    "Raft achieves consensus through leader election. A follower that hasn't heard from the leader starts an election by incrementing its term and requesting votes from other nodes."
            );

            var ingestResult = client.ingest(docs);
            System.out.printf("Ingested %d documents → %d chunks indexed.%n%n",
                    docs.size(), ingestResult.chunksAdded());

            // 3. Ask 3 questions via LangChain4j (retrieval from Python, generation via LLM)
            var questions = List.of(
                    "What is a Write-Ahead Log and which databases use it?",
                    "How does Reciprocal Rank Fusion work?",
                    "What is consistent hashing used for?"
            );

            System.out.println("Answering questions (retrieval: Python RAG, generation: LLM):\n");
            for (int i = 0; i < questions.size(); i++) {
                String q = questions.get(i);
                System.out.printf("Q%d: %s%n", i + 1, q);
                try {
                    String answer = rag.answer(q);
                    System.out.printf("A%d: %s%n%n", i + 1, answer);
                } catch (Exception e) {
                    System.out.printf("A%d: [LLM unavailable: %s]%n%n", i + 1, e.getMessage());
                }
            }

            System.out.println("Done. The Python RAG backend acted as a LangChain4j ContentRetriever.");
            System.out.println("Any Java AI application can use it identically.");
        };
    }
}
