package dev.pushkar.stream;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Go stream-processor service.
 *
 * <p>The Go processor exposes a minimal REST API for submitting events and
 * querying window results. This client wraps it with a typed Java interface,
 * mirroring the conceptual contract of a Kafka Streams input/output topic.
 *
 * <p>Kept intentionally under 60 lines to stay focused on the integration seam.
 */
public class StreamProcessorClient {

    private final RestClient http;

    public StreamProcessorClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Submit a single event to the processor.
     *
     * <p>Analogous to: {@code producer.send(new ProducerRecord<>("sensor-events", key, value))}
     * in Kafka Streams — the event enters the input topic and flows through the stream topology.
     */
    public void sendEvent(String key, double value, Instant timestamp) {
        http.post()
                .uri("/events")
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of(
                        "key", key,
                        "value", value,
                        "timestamp", timestamp.toString()
                ))
                .retrieve()
                .toBodilessEntity();
    }

    /**
     * Query aggregated window results from the processor.
     *
     * <p>Analogous to reading from a Kafka Streams {@code KTable} output topic,
     * or querying an interactive query store via {@code streams.store(...)}.
     */
    public List<WindowResultDto> queryResults(String key) {
        return http.get()
                .uri("/results/{key}", key)
                .retrieve()
                .body(WindowResultListDto.class)
                .results();
    }

    /** DTO mirroring the Go WindowResult struct. */
    public record WindowResultDto(
            String windowStart,
            String windowEnd,
            String key,
            int count,
            double sum,
            double min,
            double max,
            double avg
    ) {}

    private record WindowResultListDto(List<WindowResultDto> results) {}
}
