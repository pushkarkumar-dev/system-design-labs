package dev.pushkar.gossip;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo Spring Boot application that exercises the gossip cluster integration.
 *
 * Start the Go gossip server first:
 *   go run ./cmd/server --http-port 8080 --gossip-addr 127.0.0.1:7946
 *
 * Optionally start a second node to form a real cluster:
 *   go run ./cmd/server --http-port 8081 --gossip-addr 127.0.0.1:7947 --join 127.0.0.1:7946
 *
 * Then run this demo:
 *   mvn spring-boot:run
 *
 * The demo connects to the gossip cluster, shows current membership,
 * joins a second node if available, and watches for status changes.
 */
@SpringBootApplication
public class GossipDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(GossipDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(GossipClient gossip, ClusterHealthService health) {
        return args -> {
            System.out.println("=== SWIM Gossip Protocol — Spring Integration Demo ===\n");

            // Force an immediate membership refresh (normally runs on schedule).
            health.refreshMembers();

            // Show current cluster state.
            var members = gossip.getMembers();
            System.out.printf("Cluster has %d known members:%n", members.size());
            members.forEach(m ->
                System.out.printf("  %-30s  status=%-8s%n", m.addr(), m.status())
            );

            // Gossip statistics.
            var stats = gossip.getStats();
            System.out.printf("%nGossip stats: rounds=%d  messages=%d  members=%d%n",
                    stats.roundCount(), stats.messagesSent(), stats.memberCount());

            // Membership breakdown via ClusterHealthService.
            var live    = health.getLiveMembers();
            var suspect = health.getSuspectMembers();
            System.out.printf("%nLive members (%d):%n", live.size());
            live.forEach(m -> System.out.printf("  %s%n", m.addr()));

            if (!suspect.isEmpty()) {
                System.out.printf("%nSuspect members (%d) — probe in progress:%n", suspect.size());
                suspect.forEach(m -> System.out.printf("  %s%n", m.addr()));
            }

            // Show what Actuator health would report.
            var h = health.health();
            System.out.printf("%nActuator health status: %s%n", h.getStatus());
            h.getDetails().forEach((k, v) ->
                System.out.printf("  %s = %s%n", k, v)
            );

            System.out.println("\nDemo complete. Actuator available at /actuator/health");
        };
    }
}
