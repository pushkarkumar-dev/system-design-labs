package dev.pushkar.kafka;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.Map;

/**
 * Kafka-lite producer: wraps the broker's HTTP {@code POST /produce} endpoint
 * behind a {@code send(topic, message)} interface that mirrors
 * {@code KafkaTemplate<String, String>.send()}.
 *
 * <p><strong>Wire-compatibility lesson:</strong> Real Spring Kafka uses
 * {@code KafkaTemplate} backed by the native Kafka binary protocol
 * (TCP, port 9092). Our broker speaks HTTP — so this client wraps
 * {@link RestClient} instead. The conceptual contract is identical:
 * send a message, get back a future/result containing the assigned offset.
 * The difference is entirely in the transport layer.
 *
 * <p>If you wanted true wire compatibility (i.e., point Spring Kafka at our
 * broker without changing Java code), you would need to implement the Kafka
 * wire protocol — framing, ApiKey dispatch, Produce request/response encoding.
 * That's a separate lab. Here we show the semantic equivalence.
 *
 * <p>This class is intentionally kept under 60 lines.
 */
public class KafkaLiteProducer {

    private final RestClient http;

    public KafkaLiteProducer(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Send a message to a topic on partition 0.
     *
     * <p>Analogous to {@code kafkaTemplate.send(topic, value)}.
     *
     * @param topic   the destination topic (created automatically if absent)
     * @param message plain-text message body
     * @return the {@link ProduceResult} containing the assigned offset
     * @throws KafkaLiteException on non-2xx response or network error
     */
    public ProduceResult send(String topic, String message) {
        return send(topic, 0, message);
    }

    /**
     * Send a message to a specific topic-partition.
     *
     * <p>Analogous to {@code kafkaTemplate.send(topic, partition, null, value)}.
     */
    public ProduceResult send(String topic, int partition, String message) {
        try {
            var result = http.post()
                    .uri("/produce")
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(Map.of(
                            "topic", topic,
                            "partition", partition,
                            "message", message
                    ))
                    .retrieve()
                    .body(ProduceResult.class);

            if (result == null) throw new KafkaLiteException("broker returned null produce response");
            return result;
        } catch (RestClientException e) {
            throw new KafkaLiteException("produce failed for topic=" + topic + ": " + e.getMessage(), e);
        }
    }

    /** Immutable result of a successful produce call. */
    public record ProduceResult(String topic, int partition, long offset) {}

    public static class KafkaLiteException extends RuntimeException {
        public KafkaLiteException(String msg)                  { super(msg); }
        public KafkaLiteException(String msg, Throwable cause) { super(msg, cause); }
    }
}
