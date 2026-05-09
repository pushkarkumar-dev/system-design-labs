package dev.pushkar.gossip;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.boot.actuate.health.Status;

import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.when;

/**
 * Unit tests for ClusterHealthService.
 *
 * Tests focus on the health indicator logic — the SWIM probe state machine
 * that determines when to report UP vs DOWN to Actuator consumers.
 *
 * Key invariants under test:
 * 1. All-alive cluster reports UP with correct member counts in health detail
 * 2. Majority-dead cluster reports DOWN
 * 3. Suspect members are counted separately — they do not count as alive
 * 4. Zero members (no snapshot yet) reports DOWN
 * 5. Health detail always includes memberCount, liveCount, suspectCount, deadCount
 */
@ExtendWith(MockitoExtension.class)
class ClusterHealthServiceTest {

    @Mock
    GossipClient client;

    ClusterHealthService service;

    @BeforeEach
    void setUp() {
        var props = new GossipProperties(
                "http://localhost:8080",
                Duration.ofSeconds(30),
                0.5
        );
        service = new ClusterHealthService(client, props);
    }

    // Test 1: all members alive → UP
    @Test
    void all_alive_members_returns_UP() {
        when(client.getMembers()).thenReturn(List.of(
                new GossipClient.Member("10.0.0.1:7946", "alive", 0L),
                new GossipClient.Member("10.0.0.2:7946", "alive", 0L),
                new GossipClient.Member("10.0.0.3:7946", "alive", 0L)
        ));

        service.refreshMembers();
        var health = service.health();

        assertThat(health.getStatus()).isEqualTo(Status.UP);
        assertThat(health.getDetails().get("liveCount")).isEqualTo(3L);
        assertThat(health.getDetails().get("deadCount")).isEqualTo(0L);
        assertThat(health.getDetails().get("memberCount")).isEqualTo(3);
    }

    // Test 2: majority dead → DOWN
    @Test
    void majority_dead_returns_DOWN() {
        when(client.getMembers()).thenReturn(List.of(
                new GossipClient.Member("10.0.0.1:7946", "alive",  0L),
                new GossipClient.Member("10.0.0.2:7946", "dead",   0L),
                new GossipClient.Member("10.0.0.3:7946", "dead",   0L)
        ));

        service.refreshMembers();
        var health = service.health();

        assertThat(health.getStatus()).isEqualTo(Status.DOWN);
        assertThat(health.getDetails().get("liveCount")).isEqualTo(1L);
        assertThat(health.getDetails().get("deadCount")).isEqualTo(2L);
    }

    // Test 3: suspect members are not counted as alive
    @Test
    void suspect_members_not_counted_as_alive() {
        // 1 alive, 2 suspect — live ratio = 1/3 = 33% < 50% threshold → DOWN
        when(client.getMembers()).thenReturn(List.of(
                new GossipClient.Member("10.0.0.1:7946", "alive",   0L),
                new GossipClient.Member("10.0.0.2:7946", "suspect", 0L),
                new GossipClient.Member("10.0.0.3:7946", "suspect", 0L)
        ));

        service.refreshMembers();
        var health = service.health();

        assertThat(health.getStatus()).isEqualTo(Status.DOWN);
        assertThat(health.getDetails().get("suspectCount")).isEqualTo(2L);
        assertThat(health.getDetails().get("liveCount")).isEqualTo(1L);
    }

    // Test 4: zero members → DOWN with descriptive message
    @Test
    void zero_members_returns_DOWN() {
        when(client.getMembers()).thenReturn(List.of());

        service.refreshMembers();
        var health = service.health();

        assertThat(health.getStatus()).isEqualTo(Status.DOWN);
        assertThat(health.getDetails()).containsKey("reason");
        assertThat(health.getDetails().get("memberCount")).isEqualTo(0);
    }

    // Test 5: health detail always contains all four count fields
    @Test
    void health_detail_contains_all_count_fields() {
        when(client.getMembers()).thenReturn(List.of(
                new GossipClient.Member("10.0.0.1:7946", "alive",   0L),
                new GossipClient.Member("10.0.0.2:7946", "suspect", 0L),
                new GossipClient.Member("10.0.0.3:7946", "dead",    0L)
        ));

        service.refreshMembers();
        var details = service.health().getDetails();

        assertThat(details).containsKeys("memberCount", "liveCount", "suspectCount", "deadCount");
        assertThat(details.get("memberCount")).isEqualTo(3);
        assertThat(details.get("liveCount")).isEqualTo(1L);
        assertThat(details.get("suspectCount")).isEqualTo(1L);
        assertThat(details.get("deadCount")).isEqualTo(1L);
    }
}
