package dev.pushkar.queue;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;

/**
 * HTTP client for the message-queue Go server.
 *
 * <p>Wraps the three core SQS operations — send, receive, delete — behind
 * a typed Java interface. Under 60 lines of logic.
 *
 * <p><strong>The JMS comparison:</strong><br>
 * Spring's {@code @JmsListener} hides all of this:
 * <pre>
 *   {@code @JmsListener(destination = "orders")}
 *   public void onMessage(String body) {
 *       process(body);
 *       // Spring commits the JMS "ack" automatically after this method returns.
 *       // If the method throws, the message is redelivered (JMS sessions).
 *   }
 * </pre>
 * Behind that single annotation, the JMS container is doing exactly what this
 * class does explicitly: polling for messages, tracking the session/ack handle,
 * deleting (acking) on success, and re-enqueuing (rolling back) on failure.
 * The visibility timeout is the SQS equivalent of a JMS session acknowledgement
 * window. Making it explicit — as this class does — forces you to think about
 * what "at-least-once delivery" means in your application.
 */
public class QueueClient {

    private final RestClient http;

    public QueueClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Send a message body to the named queue.
     * Analogous to {@code SqsTemplate.send(queueName, body)} or
     * {@code JmsTemplate.convertAndSend(destination, body)}.
     *
     * @return the server-assigned MessageID
     */
    public String sendMessage(String queueName, String body) {
        try {
            var resp = http.post()
                    .uri("/queues/{name}/messages", queueName)
                    .contentType(MediaType.APPLICATION_JSON)
                    .body(Map.of("body", body))
                    .retrieve()
                    .body(SendResponse.class);
            if (resp == null) throw new QueueException("null response from server");
            return resp.messageId();
        } catch (RestClientException e) {
            throw new QueueException("sendMessage failed: " + e.getMessage(), e);
        }
    }

    /**
     * Receive up to {@code maxMessages} messages from the named queue.
     * Messages are in-flight until {@link #deleteMessage} is called or
     * {@code visibilityTimeoutSec} elapses.
     *
     * <p>Analogous to {@code SqsTemplate.receiveMany(queueName, maxMessages)} or
     * a JMS {@code MessageConsumer.receive()} call with a session transaction.
     */
    public List<QueueMessage> receiveMessages(String queueName, int maxMessages, int visibilityTimeoutSec) {
        try {
            var resp = http.get()
                    .uri("/queues/{name}/messages?maxMessages={max}&visibilityTimeout={vis}",
                            queueName, maxMessages, (double) visibilityTimeoutSec)
                    .retrieve()
                    .body(ReceiveResponse.class);
            if (resp == null || resp.messages() == null) return List.of();
            return resp.messages();
        } catch (RestClientException e) {
            throw new QueueException("receiveMessages failed: " + e.getMessage(), e);
        }
    }

    /**
     * Permanently delete an in-flight message by its receipt handle.
     * This is the SQS equivalent of a JMS session {@code acknowledge()} call.
     * If this call is not made within the visibility timeout, the message
     * reappears in the queue for another consumer.
     */
    public void deleteMessage(String queueName, String receiptHandle) {
        try {
            http.delete()
                    .uri("/queues/{name}/messages/{rh}", queueName, receiptHandle)
                    .retrieve()
                    .toBodilessEntity();
        } catch (RestClientException e) {
            throw new QueueException("deleteMessage failed: " + e.getMessage(), e);
        }
    }

    // ── Response records ──────────────────────────────────────────────────────

    public record SendResponse(String messageId) {}

    public record QueueMessage(
            String id,
            String body,
            String receiptHandle,
            int receiveCount
    ) {}

    private record ReceiveResponse(List<QueueMessage> messages) {}

    public static class QueueException extends RuntimeException {
        public QueueException(String msg)                  { super(msg); }
        public QueueException(String msg, Throwable cause) { super(msg, cause); }
    }
}
