package dev.pushkar.discovery;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.web.client.RestClient;

import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Unit tests for {@link DiscoveryClient}.
 *
 * <p>These tests do NOT start a Spring context or require a live Go registry.
 * They verify the client's record types and local logic only.
 *
 * <p>Integration tests against the live Go server are documented in the
 * lab's Run it yourself section and require {@code go run ./cmd/server} to be
 * running on localhost:8080.
 */
class DiscoveryClientTest {

    @Test
    void serviceInstance_address_concatenatesHostAndPort() {
        var inst = new DiscoveryClient.ServiceInstance(
                "pay-1", "payment-service", "10.0.0.1", "8080",
                List.of("primary"), Map.of(), null);

        assertThat(inst.address()).isEqualTo("10.0.0.1:8080");
    }

    @Test
    void serviceInstance_isImmutableRecord() {
        var inst = new DiscoveryClient.ServiceInstance(
                "id1", "svc", "host", "port",
                List.of("tag"), Map.of("k", "v"), null);

        // Records are final value types — verify field access works.
        assertThat(inst.id()).isEqualTo("id1");
        assertThat(inst.serviceName()).isEqualTo("svc");
        assertThat(inst.tags()).containsExactly("tag");
        assertThat(inst.metadata()).containsEntry("k", "v");
    }

    @Test
    void serviceInstance_emptyTagList_doesNotThrow() {
        var inst = new DiscoveryClient.ServiceInstance(
                "id2", "svc", "host", "port",
                List.of(), Map.of(), null);

        assertThat(inst.tags()).isEmpty();
    }

    @Test
    void serviceInstance_nullMetadata_isAccepted() {
        var inst = new DiscoveryClient.ServiceInstance(
                "id3", "svc", "host", "port",
                List.of(), null, null);

        assertThat(inst.metadata()).isNull();
    }

    @Test
    void discoveryProperties_defaults_areApplied() {
        // Canonical defaults via compact constructor.
        var props = new DiscoveryProperties(null, 0, null);

        assertThat(props.baseUrl()).isEqualTo("http://localhost:8080");
        assertThat(props.defaultTtlSeconds()).isEqualTo(30);
        assertThat(props.connectTimeout().toSeconds()).isEqualTo(2);
    }

    @Test
    void discoveryProperties_customValues_arePreserved() {
        var props = new DiscoveryProperties("http://registry:9090", 60, java.time.Duration.ofSeconds(5));

        assertThat(props.baseUrl()).isEqualTo("http://registry:9090");
        assertThat(props.defaultTtlSeconds()).isEqualTo(60);
        assertThat(props.connectTimeout().toSeconds()).isEqualTo(5);
    }
}
