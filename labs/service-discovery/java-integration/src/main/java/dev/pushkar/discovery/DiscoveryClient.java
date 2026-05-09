package dev.pushkar.discovery;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Go service discovery registry.
 *
 * <p>Wraps four endpoints:
 * <ul>
 *   <li>{@code POST /register}             — register a service instance
 *   <li>{@code DELETE /instances/{id}}     — deregister by ID
 *   <li>{@code GET /instances/{service}}   — list healthy instances
 *   <li>{@code PUT /instances/{id}/heartbeat} — renew TTL
 * </ul>
 *
 * <p>Built with Spring Framework 6.1's {@code RestClient} (synchronous, fluent).
 * For reactive usage, swap to {@code WebClient} from spring-boot-starter-webflux.
 *
 * <p>Compare with Spring Cloud's {@code DiscoveryClient}:
 * Spring Cloud injects a {@code LoadBalancerInterceptor} that calls
 * {@code DiscoveryClient.getInstances("payment-service")} before each request.
 * Our client makes the same lookup explicit so the mechanism is visible.
 */
public class DiscoveryClient {

    private final RestClient http;

    public DiscoveryClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    // ── Lookup ────────────────────────────────────────────────────────────────

    /** Returns all healthy instances registered for {@code serviceName}. */
    public List<ServiceInstance> getInstances(String serviceName) {
        var result = http.get()
                .uri("/instances/{svc}", serviceName)
                .retrieve()
                .body(new ParameterizedTypeReference<List<ServiceInstance>>() {});
        return result != null ? result : List.of();
    }

    // ── Registration ──────────────────────────────────────────────────────────

    /** Registers a service instance with the given TTL in seconds. */
    public void register(ServiceInstance instance, int ttlSeconds) {
        var body = Map.of(
                "id",           instance.id(),
                "serviceName",  instance.serviceName(),
                "host",         instance.host(),
                "port",         instance.port(),
                "tags",         instance.tags() != null ? instance.tags() : List.of(),
                "ttl_seconds",  ttlSeconds
        );
        http.post()
                .uri("/register")
                .contentType(MediaType.APPLICATION_JSON)
                .body(body)
                .retrieve()
                .toBodilessEntity();
    }

    /** Removes a service instance from the registry. */
    public void deregister(String instanceId) {
        http.delete()
                .uri("/instances/{id}", instanceId)
                .retrieve()
                .toBodilessEntity();
    }

    /** Renews the TTL for a registered instance. Call this on a schedule. */
    public void heartbeat(String instanceId) {
        http.put()
                .uri("/instances/{id}/heartbeat", instanceId)
                .retrieve()
                .toBodilessEntity();
    }

    // ── DTOs ──────────────────────────────────────────────────────────────────

    /**
     * A registered service instance returned by the Go registry.
     * Java 17 record — immutable, zero-boilerplate.
     */
    public record ServiceInstance(
            String id,
            String serviceName,
            String host,
            String port,
            List<String> tags,
            Map<String, String> metadata,
            Instant registeredAt
    ) {
        /** Convenience: returns "host:port" for use in HTTP clients. */
        public String address() {
            return host + ":" + port;
        }
    }
}
