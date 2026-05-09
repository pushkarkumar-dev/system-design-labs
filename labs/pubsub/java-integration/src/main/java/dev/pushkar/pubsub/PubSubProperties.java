package dev.pushkar.pubsub;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the pubsub broker client.
 *
 * <p>Set in {@code application.properties}:
 * <pre>
 *   pubsub.broker-url=http://localhost:8080
 * </pre>
 */
@ConfigurationProperties(prefix = "pubsub")
public class PubSubProperties {

    /** Base URL of the pubsub Go broker. */
    private String brokerUrl = "http://localhost:8080";

    /** Default pull timeout in milliseconds (0 = non-blocking). */
    private int pullTimeoutMs = 0;

    public String getBrokerUrl() { return brokerUrl; }
    public void setBrokerUrl(String brokerUrl) { this.brokerUrl = brokerUrl; }

    public int getPullTimeoutMs() { return pullTimeoutMs; }
    public void setPullTimeoutMs(int pullTimeoutMs) { this.pullTimeoutMs = pullTimeoutMs; }
}
