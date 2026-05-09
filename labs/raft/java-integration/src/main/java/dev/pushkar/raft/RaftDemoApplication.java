package dev.pushkar.raft;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

import java.time.Duration;
import java.util.List;
import java.util.Map;

/**
 * Demonstration application: connects to a running 3-node Raft cluster,
 * submits 100 SET commands through the Java client, then verifies all nodes
 * have identical state.
 *
 * <p>Before running, start the Go cluster:
 * <pre>
 *   cd labs/raft
 *   go run ./cmd/server --nodes=3 --base-port=8080
 * </pre>
 *
 * <p>Then run this application:
 * <pre>
 *   cd labs/raft/java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class RaftDemoApplication implements CommandLineRunner {

    private static final Logger log = LoggerFactory.getLogger(RaftDemoApplication.class);

    private final RaftClusterService cluster;
    private final List<RaftClient> clients;

    public RaftDemoApplication(RaftClusterService cluster, List<RaftClient> clients) {
        this.cluster = cluster;
        this.clients = clients;
    }

    public static void main(String[] args) {
        SpringApplication.run(RaftDemoApplication.class, args);
    }

    @Override
    public void run(String... args) throws Exception {
        System.out.println("\n=== Raft Java Client Demo ===\n");

        // 1. Wait for leader election.
        System.out.print("Waiting for leader election... ");
        String leaderUrl = cluster.waitForLeader(Duration.ofSeconds(5));
        System.out.println("leader at " + leaderUrl);

        // 2. Submit 100 SET commands.
        System.out.println("Submitting 100 SET commands through Java client...");
        int successCount = 0;
        for (int i = 0; i < 100; i++) {
            try {
                cluster.execute(String.format("SET key%d value%d", i, i));
                successCount++;
            } catch (Exception e) {
                log.warn("Command {} failed: {}", i, e.getMessage());
            }
        }
        System.out.printf("  %d / 100 commands accepted%n%n", successCount);

        // Give the cluster time to replicate.
        Thread.sleep(500);

        // 3. Verify all nodes have an identical commit index.
        System.out.println("Verifying consistency across all nodes:");
        int maxCommit = 0;
        for (RaftClient client : clients) {
            try {
                RaftClient.NodeState state = client.getState();
                System.out.printf("  node %d  state=%-10s  term=%d  commitIndex=%d%n",
                        state.id(), state.state(), state.term(), state.commitIndex());
                maxCommit = Math.max(maxCommit, state.commitIndex());
            } catch (Exception e) {
                System.out.printf("  node @ %s  UNREACHABLE%n", client.baseUrl());
            }
        }

        boolean consistent = clients.stream().allMatch(c -> {
            try {
                return c.getState().commitIndex() == maxCommit;
            } catch (Exception e) {
                return false;
            }
        });
        System.out.printf("%nConsistency check: %s%n", consistent ? "PASS" : "FAIL");

        // 4. Print the first few log entries from the leader.
        System.out.println("\nFirst 5 log entries from leader:");
        try {
            cluster.findLeader().ifPresent(leader -> {
                List<RaftClient.LogEntry> entries = leader.getLog();
                entries.stream().limit(5).forEach(e ->
                        System.out.printf("  [term=%d] %s%n", e.term(), e.command()));
                if (entries.size() > 5) {
                    System.out.printf("  ... and %d more%n", entries.size() - 5);
                }
            });
        } catch (Exception e) {
            log.warn("Could not read log: {}", e.getMessage());
        }

        System.out.println("\nDemo complete.");
    }
}
