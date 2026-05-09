package dev.pushkar.gossip;

import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.scheduling.annotation.Scheduled;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;

/**
 * Monitors the gossip cluster and exposes its health via Spring Boot Actuator.
 *
 * <p>Polls the gossip server's {@code /members} endpoint every 30 seconds
 * (configurable via {@code gossip.poll-interval}) and caches the result.
 * The Actuator health endpoint at {@code /actuator/health} reports:
 * <ul>
 *   <li>UP if more than 50% of known members are Alive
 *   <li>DOWN otherwise (majority dead or no members known)
 * </ul>
 *
 * <p>Spring Cloud Consul uses a very similar pattern underneath its
 * {@code DiscoveryClient} implementation — this class makes that explicit.
 */
@Service
public class ClusterHealthService implements HealthIndicator {

    private final GossipClient client;
    private final GossipProperties props;

    // Snapshot updated by the scheduler; reads need no lock (CopyOnWriteArrayList).
    private volatile List<GossipClient.Member> lastSnapshot = List.of();

    public ClusterHealthService(GossipClient client, GossipProperties props) {
        this.client = client;
        this.props = props;
    }

    // ── Scheduled polling ─────────────────────────────────────────────────────

    /**
     * Refresh the membership snapshot from the gossip server.
     * Runs every {@code gossip.poll-interval} (default 30 seconds).
     */
    @Scheduled(fixedDelayString = "#{@gossipProperties.pollIntervalMillis()}")
    public void refreshMembers() {
        try {
            lastSnapshot = client.getMembers();
        } catch (Exception e) {
            // Keep the stale snapshot — health() will report DOWN.
        }
    }

    // ── Member queries ────────────────────────────────────────────────────────

    /** Returns all members currently reported as Alive. */
    public List<GossipClient.Member> getLiveMembers() {
        return lastSnapshot.stream()
                .filter(GossipClient.Member::isAlive)
                .toList();
    }

    /** Returns all members currently in Suspect state. */
    public List<GossipClient.Member> getSuspectMembers() {
        return lastSnapshot.stream()
                .filter(GossipClient.Member::isSuspect)
                .toList();
    }

    // ── HealthIndicator ───────────────────────────────────────────────────────

    /**
     * Exposes cluster membership counts to {@code /actuator/health}.
     *
     * <p>UP when: {@code liveCount / memberCount > minLiveRatio} (default 0.5).
     * DOWN when: no members known, or live ratio falls below threshold.
     */
    @Override
    public Health health() {
        var members = lastSnapshot;
        int memberCount = members.size();

        if (memberCount == 0) {
            return Health.down()
                    .withDetail("reason", "no cluster members known")
                    .withDetail("memberCount", 0)
                    .build();
        }

        long liveCount    = members.stream().filter(GossipClient.Member::isAlive).count();
        long suspectCount = members.stream().filter(GossipClient.Member::isSuspect).count();
        long deadCount    = members.stream().filter(GossipClient.Member::isDead).count();

        double liveRatio = (double) liveCount / memberCount;
        boolean healthy  = liveRatio > props.minLiveRatio();

        var builder = healthy ? Health.up() : Health.down();
        return builder
                .withDetail("memberCount",  memberCount)
                .withDetail("liveCount",    liveCount)
                .withDetail("suspectCount", suspectCount)
                .withDetail("deadCount",    deadCount)
                .withDetail("liveRatio",    String.format("%.0f%%", liveRatio * 100))
                .build();
    }
}
