package dev.pushkar.pubsub;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.Base64;
import java.util.Map;

/**
 * PubSubClient wraps the pub/sub broker's HTTP API behind a clean Java interface.
 *
 * <p>Analogous to how {@code AmazonSNS.publish()} abstracts AWS SNS, this
 * client hides the HTTP wire format and provides strongly-typed publish and
 * subscribe methods.
 *
 * <p>Intentionally kept under 60 lines.
 */
public class PubSubClient {

    private final RestClient http;

    public PubSubClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /** Create a topic on the broker (idempotent). */
    public void createTopic(String topic) {
        http.post().uri("/topics")
                .contentType(MediaType.APPLICATION_JSON)
                .body(Map.of("name", topic))
                .retrieve().toBodilessEntity();
    }

    /**
     * Publish a message to a topic with optional attributes.
     * Body is UTF-8 encoded and base64-transmitted in JSON.
     *
     * @return the assigned message ID
     */
    public String publish(String topic, String body, Map<String, String> attributes) {
        try {
            var payload = Map.of(
                    "body", Base64.getEncoder().encodeToString(body.getBytes()),
                    "attributes", attributes != null ? attributes : Map.of()
            );
            @SuppressWarnings("unchecked")
            var resp = (Map<String, Object>) http.post()
                    .uri("/topics/{topic}/publish", topic)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(payload)
                    .retrieve()
                    .body(Map.class);
            if (resp == null) throw new PubSubException("null publish response");
            return (String) resp.get("messageId");
        } catch (RestClientException e) {
            throw new PubSubException("publish failed: " + e.getMessage(), e);
        }
    }

    /**
     * Create a pull subscription on the broker.
     *
     * @return the subscription ID assigned by the broker
     */
    public String createSubscription(String topic) {
        try {
            @SuppressWarnings("unchecked")
            var resp = (Map<String, Object>) http.post()
                    .uri("/subscriptions")
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(Map.of("topic", topic))
                    .retrieve()
                    .body(Map.class);
            if (resp == null) throw new PubSubException("null subscribe response");
            return (String) resp.get("subscriptionId");
        } catch (RestClientException e) {
            throw new PubSubException("createSubscription failed: " + e.getMessage(), e);
        }
    }

    /** Runtime exception for broker communication errors. */
    public static class PubSubException extends RuntimeException {
        public PubSubException(String msg)                  { super(msg); }
        public PubSubException(String msg, Throwable cause) { super(msg, cause); }
    }
}
