package dev.pushkar.raft;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicReference;

/**
 * Spring service that wraps the Raft cluster and implements the
 * <em>follower-redirect</em> pattern required by every Raft client.
 *
 * <h2>The problem</h2>
 * Raft only allows writes through the leader.  But the leader can change
 * at any time (crash, network partition, term change).  A naive client
 * hardcoding one node URL will fail whenever leadership moves.
 *
 * <h2>The pattern</h2>
 * <ol>
 *   <li>Cache the current leader URL.</li>
 *   <li>Submit the command to the cached leader.</li>
 *   <li>If the node returns HTTP 503 ("not the leader"), scan all known nodes
 *       to find the new leader and retry once.</li>
 *   <li>If still no leader, throw.</li>
 * </ol>
 *
 * <p>This is the same pattern used by etcd's Go client, TiKV's Java client,
 * and every serious Raft client library.
 */
@Service
public class RaftClusterService {

    private static final Logger log = LoggerFactory.getLogger(RaftClusterService.class);

    private final List<RaftClient> clients;
    private final RaftProperties props;

    /** Cached leader client; refreshed whenever a 503 is observed. */
    private final AtomicReference<RaftClient> cachedLeader = new AtomicReference<>();
    private volatile Instant leaderCachedAt = Instant.EPOCH;

    public RaftClusterService(List<RaftClient> clients, RaftProperties props) {
        this.clients = clients;
        this.props = props;
    }

    // ── Public API ────────────────────────────────────────────────────────────

    /**
     * Submits a command to the Raft cluster, transparently routing to the leader
     * and retrying once on a redirect (503).
     *
     * @param command the command string (e.g. "SET key value")
     * @return the {@link RaftClient.CommandResult} from the leader
     * @throws IllegalStateException if no leader is available
     */
    public RaftClient.CommandResult execute(String command) {
        RaftClient leader = leaderClient();
        RaftClient.CommandResult result = leader.submitCommand(command);

        if (!result.accepted() && "not the leader".equals(result.error())) {
            log.debug("Got 503 from {}; refreshing leader cache", leader.baseUrl());
            invalidateLeaderCache();
            leader = leaderClient(); // re-discover
            result = leader.submitCommand(command);
        }

        if (!result.accepted()) {
            throw new IllegalStateException("Command rejected by cluster: " + result.error());
        }
        return result;
    }

    /**
     * Returns the node that currently identifies itself as the leader, or
     * {@link Optional#empty()} if none is found.
     */
    public Optional<RaftClient> findLeader() {
        for (RaftClient c : clients) {
            try {
                RaftClient.NodeState state = c.getState();
                if (state != null && state.isLeader()) {
                    return Optional.of(c);
                }
            } catch (Exception e) {
                log.trace("Node {} unreachable: {}", c.baseUrl(), e.getMessage());
            }
        }
        return Optional.empty();
    }

    /**
     * Blocks until a leader is elected or the timeout expires.
     *
     * @param timeout maximum wait duration
     * @return the leader's base URL
     * @throws IllegalStateException if no leader is found within the timeout
     */
    public String waitForLeader(Duration timeout) {
        Instant deadline = Instant.now().plus(timeout);
        while (Instant.now().isBefore(deadline)) {
            Optional<RaftClient> leader = findLeader();
            if (leader.isPresent()) {
                return leader.get().baseUrl();
            }
            try {
                Thread.sleep(50);
            } catch (InterruptedException ex) {
                Thread.currentThread().interrupt();
                throw new IllegalStateException("Interrupted while waiting for leader");
            }
        }
        throw new IllegalStateException("No leader elected within " + timeout);
    }

    // ── Internal ──────────────────────────────────────────────────────────────

    /** Returns the cached leader client, refreshing if stale or absent. */
    private RaftClient leaderClient() {
        RaftClient cached = cachedLeader.get();
        Duration ttl = props.leaderCacheTtl();
        if (cached != null && Instant.now().isBefore(leaderCachedAt.plus(ttl))) {
            return cached;
        }
        return refreshLeaderCache();
    }

    /** Polls all nodes and caches the leader. */
    private RaftClient refreshLeaderCache() {
        Optional<RaftClient> leader = findLeader();
        RaftClient c = leader.orElseThrow(
                () -> new IllegalStateException("No leader available in cluster"));
        cachedLeader.set(c);
        leaderCachedAt = Instant.now();
        log.debug("Leader cache updated: {}", c.baseUrl());
        return c;
    }

    /** Invalidates the cached leader so the next call triggers re-discovery. */
    private void invalidateLeaderCache() {
        cachedLeader.set(null);
        leaderCachedAt = Instant.EPOCH;
    }
}
