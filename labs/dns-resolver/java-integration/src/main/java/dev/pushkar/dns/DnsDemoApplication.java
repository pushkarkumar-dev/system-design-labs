package dev.pushkar.dns;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;

/**
 * Demo application showing:
 * 1. Resolve 10 domains via our Go resolver (warm cache after first batch)
 * 2. Show cache hits on repeated queries (second batch)
 * 3. Clear cache — next queries recurse from root again
 * 4. Compare our resolver vs dnsjava system resolver for a single domain
 *
 * <p>Start the Go server first:
 * <pre>
 *   cd labs/dns-resolver
 *   go run ./cmd/server --udp 0.0.0.0:5300 --admin 0.0.0.0:5380
 * </pre>
 *
 * <p>Then run:
 * <pre>
 *   cd java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class DnsDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(DnsDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(DnsAdminClient admin, DnsJavaComparison comparison) {
        return args -> {
            System.out.println("=== DNS Resolver Spring Integration Demo ===\n");

            // ── Health check ─────────────────────────────────────────────────
            try {
                var health = admin.health();
                System.out.println("Go resolver health: " + health.status());
            } catch (Exception e) {
                System.out.println("Go resolver unreachable — start it first.");
                System.out.println("  go run ./cmd/server --udp 0.0.0.0:5300 --admin 0.0.0.0:5380");
                return;
            }

            // ── Stats before any queries ─────────────────────────────────────
            var before = admin.getStats();
            System.out.printf("Stats before: queries=%d  hits=%d  misses=%d%n%n",
                    before.queries(), before.cacheHits(), before.cacheMisses());

            // ── Resolve 10 domains (local zone — these answer immediately) ───
            var domains = List.of(
                    "example.com", "lab.example.com", "alias.example.com",
                    "example.com", "example.com",   // repeated — will be zone hits
                    "lab.example.com", "lab.example.com",
                    "alias.example.com", "alias.example.com", "example.com"
            );

            System.out.println("Resolving 10 queries (includes repeats to show cache effect):");
            for (String domain : domains) {
                try {
                    var ips = comparison.resolveWithOurResolver(domain);
                    System.out.printf("  %-30s → %s%n", domain, ips.isEmpty() ? "NXDOMAIN" : ips);
                } catch (Exception e) {
                    System.out.printf("  %-30s → error: %s%n", domain, e.getMessage());
                }
            }

            // ── Stats after queries ───────────────────────────────────────────
            var after = admin.getStats();
            System.out.printf("%nStats after:  queries=%d  hits=%d  misses=%d%n",
                    after.queries(), after.cacheHits(), after.cacheMisses());

            // ── Cache contents ────────────────────────────────────────────────
            var cacheEntries = admin.getCache();
            System.out.printf("Live cache entries: %d%n%n", cacheEntries.size());

            // ── Clear cache and show that next query goes back to resolution ──
            System.out.println("Clearing cache...");
            admin.clearCache();
            var afterFlush = admin.getCache();
            System.out.printf("Cache entries after flush: %d%n%n", afterFlush.size());

            // ── dnsjava comparison (only for domains in local zone) ───────────
            System.out.println("dnsjava comparison for 'example.com':");
            try {
                var ourResult = comparison.resolveWithOurResolver("example.com");
                System.out.printf("  Our resolver:    %s%n", ourResult);
            } catch (Exception e) {
                System.out.printf("  Our resolver:    error: %s%n", e.getMessage());
            }
            try {
                var sysResult = comparison.resolveWithSystemResolver("example.com");
                System.out.printf("  System resolver: %s%n", sysResult);
            } catch (Exception e) {
                System.out.printf("  System resolver: error: %s%n", e.getMessage());
            }
            try {
                var jdkResult = comparison.resolveWithJdkResolver("example.com");
                System.out.printf("  JDK resolver:    %s%n%n", jdkResult);
            } catch (Exception e) {
                System.out.printf("  JDK resolver:    error: %s%n%n", e.getMessage());
            }

            System.out.println("Demo complete.");
        };
    }
}
