package dev.pushkar.raft;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;
import static org.mockito.Mockito.*;

/**
 * Unit tests for {@link RaftClusterService}.
 *
 * These tests use Mockito to stub individual {@link RaftClient} instances so
 * no real Go process is required.  Six scenarios are covered:
 * <ol>
 *   <li>Command routes to the leader successfully.</li>
 *   <li>Follower 503 causes the client to find the leader and retry.</li>
 *   <li>Leader cache is invalidated after a 503 so the next lookup re-polls.</li>
 *   <li>waitForLeader times out gracefully when no leader exists.</li>
 *   <li>All-nodes-down throws immediately.</li>
 *   <li>CommandResult deserialization: accepted=false propagates error message.</li>
 * </ol>
 */
@ExtendWith(MockitoExtension.class)
class RaftClusterServiceTest {

    @Mock RaftClient node0;
    @Mock RaftClient node1;
    @Mock RaftClient node2;

    RaftProperties props;
    RaftClusterService service;

    @BeforeEach
    void setUp() {
        props = new RaftProperties(
                List.of("http://localhost:8080", "http://localhost:8081", "http://localhost:8082"),
                Duration.ofSeconds(5),
                Duration.ofSeconds(10));
        service = new RaftClusterService(List.of(node0, node1, node2), props);
    }

    // ── 1. Happy path ─────────────────────────────────────────────────────────

    @Test
    void commandRoutesToLeader() {
        // node1 is the leader.
        when(node0.getState()).thenReturn(new RaftClient.NodeState(0, "Follower", 3, 5, 5, false));
        when(node1.getState()).thenReturn(new RaftClient.NodeState(1, "Leader",   3, 5, 5, true));
        when(node1.submitCommand("SET x 1"))
                .thenReturn(new RaftClient.CommandResult(true, 1, null));

        RaftClient.CommandResult result = service.execute("SET x 1");

        assertThat(result.accepted()).isTrue();
        assertThat(result.nodeId()).isEqualTo(1);
        verify(node1).submitCommand("SET x 1");
        // node2 should never be asked.
        verify(node2, never()).submitCommand(anyString());
    }

    // ── 2. Follower-redirect ──────────────────────────────────────────────────

    @Test
    void followerRedirectCausesLeaderDiscoveryAndRetry() {
        // First poll: node0 thinks node1 is leader; we contact node1 initially.
        when(node0.getState()).thenReturn(new RaftClient.NodeState(0, "Follower", 3, 5, 5, false));
        when(node1.getState()).thenReturn(new RaftClient.NodeState(1, "Leader",   3, 5, 5, true));
        when(node2.getState()).thenReturn(new RaftClient.NodeState(2, "Follower", 3, 5, 5, false));

        // First submit attempt → node1 returns 503 (leadership just changed).
        // Second attempt (after cache refresh) → node1 is leader again.
        when(node1.submitCommand("DEL old"))
                .thenReturn(new RaftClient.CommandResult(false, 1, "not the leader"))
                .thenReturn(new RaftClient.CommandResult(true,  1, null));

        RaftClient.CommandResult result = service.execute("DEL old");

        assertThat(result.accepted()).isTrue();
        verify(node1, times(2)).submitCommand("DEL old");
    }

    // ── 3. Leader cache invalidation ──────────────────────────────────────────

    @Test
    void leaderCacheInvalidatedAfter503() {
        // Initial state: node0 is leader.
        when(node0.getState()).thenReturn(new RaftClient.NodeState(0, "Leader",   3, 10, 10, true));
        when(node0.submitCommand("SET a 1"))
                .thenReturn(new RaftClient.CommandResult(false, 0, "not the leader"));

        // After 503 the service re-polls; node2 is the new leader.
        when(node1.getState()).thenReturn(new RaftClient.NodeState(1, "Follower", 4, 10, 10, false));
        when(node2.getState()).thenReturn(new RaftClient.NodeState(2, "Leader",   4, 10, 10, true));
        when(node2.submitCommand("SET a 1"))
                .thenReturn(new RaftClient.CommandResult(true, 2, null));

        RaftClient.CommandResult result = service.execute("SET a 1");

        assertThat(result.accepted()).isTrue();
        assertThat(result.nodeId()).isEqualTo(2);
        // node0 is the original "leader" — submits once before 503.
        verify(node0).submitCommand("SET a 1");
        // node2 is the real leader after re-discovery.
        verify(node2).submitCommand("SET a 1");
    }

    // ── 4. waitForLeader timeout ──────────────────────────────────────────────

    @Test
    void waitForLeaderTimesOutWhenNoLeaderExists() {
        // All nodes return Follower state.
        when(node0.getState()).thenReturn(new RaftClient.NodeState(0, "Follower", 1, 0, 0, false));
        when(node1.getState()).thenReturn(new RaftClient.NodeState(1, "Follower", 1, 0, 0, false));
        when(node2.getState()).thenReturn(new RaftClient.NodeState(2, "Follower", 1, 0, 0, false));

        assertThatThrownBy(() -> service.waitForLeader(Duration.ofMillis(100)))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("No leader elected");
    }

    // ── 5. All nodes down ─────────────────────────────────────────────────────

    @Test
    void allNodesDownThrowsOnExecute() {
        when(node0.getState()).thenThrow(new RuntimeException("connection refused"));
        when(node1.getState()).thenThrow(new RuntimeException("connection refused"));
        when(node2.getState()).thenThrow(new RuntimeException("connection refused"));

        assertThatThrownBy(() -> service.execute("SET z 9"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("No leader available");
    }

    // ── 6. CommandResult deserialization ──────────────────────────────────────

    @Test
    void commandResultPreservesErrorMessage() {
        when(node0.getState()).thenReturn(new RaftClient.NodeState(0, "Leader", 5, 20, 20, true));
        when(node0.submitCommand("UNKNOWN cmd"))
                .thenReturn(new RaftClient.CommandResult(false, 0, "unsupported command"));

        assertThatThrownBy(() -> service.execute("UNKNOWN cmd"))
                .isInstanceOf(IllegalStateException.class)
                .hasMessageContaining("unsupported command");
    }
}
