package dev.pushkar.discovery;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;
import java.util.Map;

/**
 * Demo application: registers 3 mock services, resolves them round-robin,
 * deregisters one, and shows the registry adapts.
 *
 * <p>Requires the Go registry to be running:
 * <pre>
 *   cd labs/service-discovery
 *   go run ./cmd/server --port 8080 --ttl 30s
 * </pre>
 *
 * <p>Then in a separate terminal:
 * <pre>
 *   cd labs/service-discovery/java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class DiscoveryDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(DiscoveryDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(DiscoveryClient client, DiscoveryProperties props) {
        return args -> {
            System.out.println("\n=== Service Discovery — Spring Integration Demo ===\n");

            // ── Step 1: Register 3 payment-service instances ──────────────────
            var instances = List.of(
                    new DiscoveryClient.ServiceInstance(
                            "payment-1", "payment-service", "10.0.0.1", "8080",
                            List.of("primary"), Map.of("region", "us-east"), null),
                    new DiscoveryClient.ServiceInstance(
                            "payment-2", "payment-service", "10.0.0.2", "8080",
                            List.of("replica"), Map.of("region", "us-west"), null),
                    new DiscoveryClient.ServiceInstance(
                            "payment-3", "payment-service", "10.0.0.3", "8080",
                            List.of("replica"), Map.of("region", "eu-west"), null)
            );

            System.out.println("Registering 3 payment-service instances...");
            for (var inst : instances) {
                client.register(inst, props.defaultTtlSeconds());
                System.out.printf("  Registered: %s (%s)%n", inst.id(), inst.address());
            }

            // ── Step 2: Lookup and round-robin resolve ─────────────────────────
            System.out.println("\nLooking up payment-service (round-robin over 6 calls):");
            var alive = client.getInstances("payment-service");
            System.out.printf("  Registry reports %d healthy instances%n", alive.size());

            for (int i = 0; i < 6; i++) {
                var picked = alive.get(i % alive.size());
                System.out.printf("  Call %d → %s (%s)%n", i + 1, picked.id(), picked.address());
            }

            // ── Step 3: Deregister one instance ──────────────────────────────
            System.out.println("\nDeregistering payment-2...");
            client.deregister("payment-2");

            var remaining = client.getInstances("payment-service");
            System.out.printf("  Registry now reports %d healthy instances:%n", remaining.size());
            for (var inst : remaining) {
                System.out.printf("    %s (%s)%n", inst.id(), inst.address());
            }

            // ── Step 4: Heartbeat demonstration ───────────────────────────────
            System.out.println("\nSending heartbeat for payment-1...");
            client.heartbeat("payment-1");
            System.out.println("  Heartbeat sent — TTL renewed.");

            // ── Step 5: Cleanup ───────────────────────────────────────────────
            System.out.println("\nCleaning up: deregistering all instances...");
            client.deregister("payment-1");
            client.deregister("payment-3");
            System.out.println("  Done.\n");

            System.out.println("=== Demo complete ===");
            System.out.println("Compare with Spring Cloud DiscoveryClient:");
            System.out.println("  SpringCloudDiscoveryComparison.java shows the Eureka equivalent.");
        };
    }
}
