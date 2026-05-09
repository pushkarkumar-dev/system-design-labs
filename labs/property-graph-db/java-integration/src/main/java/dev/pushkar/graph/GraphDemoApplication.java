package dev.pushkar.graph;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.Map;

/**
 * Spring Boot 3.3 demo: run the same Cypher queries against Neo4j that our
 * Rust Cypher-lite engine supports. Shows the Neo4j driver API in practice.
 *
 * Prerequisites:
 *   - Neo4j 5.x running on bolt://localhost:7687 (or docker run neo4j)
 *   - Seed data: run seed.cypher in Neo4j Browser first
 *
 * Run:
 *   cd labs/property-graph-db/java-integration
 *   mvn spring-boot:run
 */
@SpringBootApplication
public class GraphDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(GraphDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(GraphClient client) {
        return args -> {
            System.out.println("\n=== Property Graph Database — Java/Neo4j Demo ===\n");

            System.out.println("1. MATCH (n:Person) RETURN n");
            System.out.println("   [Rust equiv: graph.find_nodes(\"Person\") with label index]");
            try {
                var persons = client.findAllPersons();
                persons.forEach(row -> System.out.println("   " + row));
            } catch (Exception e) {
                System.out.println("   [Neo4j not running — expected in CI: " + e.getMessage() + "]");
            }
            System.out.println();

            System.out.println("2. MATCH (n:Person {name: \"Alice\"}) RETURN n");
            System.out.println("   [Rust equiv: PropertyIndex.lookup(\"Person\", \"name\", \"Alice\")]");
            try {
                var result = client.findPersonByName("Alice");
                result.forEach(row -> System.out.println("   " + row));
            } catch (Exception e) {
                System.out.println("   [Neo4j not running — expected in CI: " + e.getMessage() + "]");
            }
            System.out.println();

            System.out.println("3. MATCH (n:Person)-[:KNOWS]->(m:Person) WHERE n.name = \"Alice\" RETURN m");
            System.out.println("   [Rust equiv: executor.execute(PathMatch{rel_type: KNOWS}, graph)]");
            System.out.println("   Neo4j uses index-free adjacency: O(1) per hop via relationship record chain.");
            System.out.println("   Our Rust impl uses HashMap lookup: O(1) amortized but with cache misses.");
            try {
                var friends = client.findFriends("Alice");
                friends.forEach(row -> System.out.println("   " + row));
            } catch (Exception e) {
                System.out.println("   [Neo4j not running — expected in CI: " + e.getMessage() + "]");
            }
            System.out.println();

            System.out.println("4. MATCH (n:Person)-[:KNOWS*1..3]->(m:Person) WHERE n.name = \"Alice\" RETURN m");
            System.out.println("   [Rust equiv: dfs_traverse(graph, alice_id, \"KNOWS\", 1, 3)]");
            try {
                var hops = client.findFriendsUpTo3Hops("Alice");
                System.out.println("   Found " + hops.size() + " friends within 3 hops:");
                hops.forEach(row -> System.out.println("   " + row.get("name")));
            } catch (Exception e) {
                System.out.println("   [Neo4j not running — expected in CI: " + e.getMessage() + "]");
            }
            System.out.println();

            System.out.println("Key difference: Neo4j's variable-length path uses relationship pointer");
            System.out.println("chains (index-free adjacency). Our DFS uses a HashMap<NodeId, Vec<EdgeId>>");
            System.out.println("which adds one HashMap lookup per hop instead of one pointer dereference.");
            System.out.println("At 1M nodes this is imperceptible; at 1B nodes Neo4j wins decisively.");
        };
    }
}
