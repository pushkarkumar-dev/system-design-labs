package dev.pushkar.graph;

import org.neo4j.driver.Driver;
import org.neo4j.driver.Record;
import org.neo4j.driver.Result;
import org.neo4j.driver.Session;
import org.neo4j.driver.Values;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.Map;

/**
 * GraphClient wraps the Neo4j Java Driver to run raw Cypher queries —
 * the same queries our Rust toy supports in Cypher-lite form.
 *
 * This is the "production equivalent" of the Rust executor:
 *   - Rust: parse() + execute() on a HashMap-backed adjacency list
 *   - Neo4j: driver.session().run(cypher) on native graph storage with
 *             index-free adjacency, O(1) per hop regardless of graph size
 *
 * Line count: ~50 lines (service-client target)
 */
@Component
public class GraphClient {

    private final Driver driver;

    public GraphClient(Driver driver) {
        this.driver = driver;
    }

    /**
     * MATCH (n:Person) RETURN n
     * Equivalent to: graph.find_nodes("Person") with label index.
     */
    public List<Map<String, Object>> findAllPersons() {
        try (Session session = driver.session()) {
            Result result = session.run("MATCH (n:Person) RETURN n.name AS name, n.age AS age");
            return result.list(Record::asMap);
        }
    }

    /**
     * MATCH (n:Person {name: $name}) RETURN n
     * Equivalent to: property index lookup on :Person(name).
     */
    public List<Map<String, Object>> findPersonByName(String name) {
        try (Session session = driver.session()) {
            Result result = session.run(
                "MATCH (n:Person {name: $name}) RETURN n.name AS name, n.age AS age",
                Values.parameters("name", name)
            );
            return result.list(Record::asMap);
        }
    }

    /**
     * MATCH (n:Person)-[:KNOWS]->(m:Person) WHERE n.name = $name RETURN m
     * Equivalent to: executor.execute(PathMatch{rel_type: KNOWS}, graph).
     *
     * Neo4j traverses this in O(degree) per node using index-free adjacency —
     * no index needed, relationship records chain directly to each other.
     */
    public List<Map<String, Object>> findFriends(String name) {
        try (Session session = driver.session()) {
            Result result = session.run(
                "MATCH (n:Person)-[:KNOWS]->(m:Person) WHERE n.name = $name RETURN m.name AS name",
                Values.parameters("name", name)
            );
            return result.list(Record::asMap);
        }
    }

    /**
     * MATCH (n:Person)-[:KNOWS*1..3]->(m:Person) WHERE n.name = $name RETURN m
     * Equivalent to: dfs_traverse(graph, alice_id, "KNOWS", 1, 3).
     *
     * Neo4j handles this with a built-in variable-length path algorithm
     * that uses relationship pointers, not DFS on a HashMap.
     */
    public List<Map<String, Object>> findFriendsUpTo3Hops(String name) {
        try (Session session = driver.session()) {
            Result result = session.run(
                "MATCH (n:Person)-[:KNOWS*1..3]->(m:Person) WHERE n.name = $name RETURN DISTINCT m.name AS name",
                Values.parameters("name", name)
            );
            return result.list(Record::asMap);
        }
    }
}
