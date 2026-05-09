package dev.pushkar.kafka;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.core.ParameterizedTypeReference;
import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.util.Collections;
import java.util.List;
import java.util.Map;

/**
 * Kafka-lite consumer: polls the broker's HTTP {@code GET /consume} endpoint
 * in a loop, simulating {@code @KafkaListener} semantics.
 *
 * <h2>At-least-once vs at-most-once processing</h2>
 *
 * <p>The order of commit vs process determines delivery guarantees:
 *
 * <pre>
 * At-most-once (commit BEFORE process):
 *   1. Fetch messages at committed_offset
 *   2. Commit offset = last_offset + 1      ← offset advanced
 *   3. Process messages
 *   4. If process crashes → messages LOST (offset already moved past them)
 *
 * At-least-once (commit AFTER process):
 *   1. Fetch messages at committed_offset
 *   2. Process messages
 *   3. Commit offset = last_offset + 1      ← offset advanced
 *   4. If process crashes before step 3 → messages re-consumed on restart
 *      → same message processed MORE THAN ONCE (idempotency required)
 * </pre>
 *
 * <p>This class implements at-least-once by default: it does NOT auto-commit
 * until {@link #commitOffset()} is called explicitly (or {@code autoCommit=true}).
 * Restarting the consumer and calling {@link #poll(String, int, int)} again
 * will re-deliver any uncommitted messages.
 */
public class KafkaLiteConsumer {

    private static final Logger log = LoggerFactory.getLogger(KafkaLiteConsumer.class);

    private final RestClient http;
    private final String groupId;
    private final String memberID;
    private final boolean autoCommit;

    // Tracks the current fetch cursor (not yet committed to the broker).
    private long localOffset = 0;

    public KafkaLiteConsumer(String baseUrl, String groupId, String memberID, boolean autoCommit) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
        this.groupId = groupId;
        this.memberID = memberID;
        this.autoCommit = autoCommit;

        // Load the committed offset from the broker so restarts resume correctly.
        this.localOffset = fetchCommittedOffset("", 0);
    }

    /**
     * Poll for up to {@code maxMessages} records from the topic starting at
     * the current local offset.
     *
     * <p>If {@code autoCommit=true}, the offset is advanced and committed after
     * each successful poll (equivalent to Kafka's {@code enable.auto.commit=true}).
     *
     * <p>If {@code autoCommit=false} (default), the offset cursor advances locally
     * but is NOT written to the broker until {@link #commitOffset()} is called.
     * A restart before commit will re-deliver the same messages.
     *
     * @param topic       topic to consume from
     * @param partition   partition index (0 for single-partition topics)
     * @param maxMessages maximum number of messages to return per poll
     * @return list of consumed messages (may be empty)
     */
    public List<ConsumedMessage> poll(String topic, int partition, int maxMessages) {
        try {
            var response = http.get()
                    .uri("/consume?topic={topic}&partition={partition}&offset={offset}&limit={limit}",
                            topic, partition, localOffset, maxMessages)
                    .retrieve()
                    .body(new ParameterizedTypeReference<Map<String, List<Map<String, Object>>>>() {});

            if (response == null || !response.containsKey("messages")) {
                return Collections.emptyList();
            }

            List<Map<String, Object>> raw = response.get("messages");
            if (raw == null || raw.isEmpty()) {
                return Collections.emptyList();
            }

            List<ConsumedMessage> messages = raw.stream().map(m -> {
                long offset = ((Number) m.get("offset")).longValue();
                String payload = (String) m.get("payload");
                return new ConsumedMessage(offset, payload);
            }).toList();

            // Advance local cursor past the last received message.
            long lastOffset = messages.get(messages.size() - 1).offset();
            localOffset = lastOffset + 1;

            if (autoCommit) {
                doCommit(topic, partition, localOffset);
            }

            return messages;

        } catch (Exception e) {
            log.warn("poll failed: {}", e.getMessage());
            return Collections.emptyList();
        }
    }

    /**
     * Manually commit the current local offset to the broker.
     *
     * <p>Call this after successfully processing a batch of messages to ensure
     * at-least-once semantics. If you never call this, every restart re-delivers
     * from the last committed position.
     *
     * @param topic     the topic the offset belongs to
     * @param partition the partition index
     */
    public void commitOffset(String topic, int partition) {
        doCommit(topic, partition, localOffset);
    }

    /**
     * Send a heartbeat to keep this member's slot in the consumer group.
     * Must be called at least once every 3 seconds or the broker ejects the member.
     */
    public void heartbeat() {
        try {
            http.post()
                    .uri("/groups/{groupId}/heartbeat?memberId={memberId}", groupId, memberID)
                    .retrieve()
                    .toBodilessEntity();
        } catch (Exception e) {
            log.warn("heartbeat failed for group={}, member={}: {}", groupId, memberID, e.getMessage());
        }
    }

    /** Returns the current local (uncommitted) read cursor. */
    public long localOffset() {
        return localOffset;
    }

    // ── Private helpers ───────────────────────────────────────────────────────

    private void doCommit(String topic, int partition, long offset) {
        try {
            http.post()
                    .uri("/groups/{groupId}/commit", groupId)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(Map.of("topic", topic, "partition", partition, "offset", offset))
                    .retrieve()
                    .toBodilessEntity();
        } catch (Exception e) {
            log.warn("commit failed: {}", e.getMessage());
        }
    }

    private long fetchCommittedOffset(String topic, int partition) {
        if (topic == null || topic.isEmpty()) {
            return 0; // No topic known yet — start from the beginning.
        }
        try {
            var resp = http.get()
                    .uri("/groups/{groupId}/offset?topic={topic}&partition={partition}",
                            groupId, topic, partition)
                    .retrieve()
                    .body(new ParameterizedTypeReference<Map<String, Object>>() {});

            if (resp == null) return 0;
            Object off = resp.get("offset");
            if (off instanceof Number n) return n.longValue();
            return 0;
        } catch (Exception e) {
            return 0;
        }
    }

    /** A single consumed record. */
    public record ConsumedMessage(long offset, String payload) {}
}
