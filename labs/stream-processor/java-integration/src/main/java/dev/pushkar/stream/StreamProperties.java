package dev.pushkar.stream;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the stream processor client.
 * Maps to the {@code stream-processor.*} namespace in application.properties.
 */
@ConfigurationProperties(prefix = "stream-processor")
public class StreamProperties {

    /** Base URL of the Go stream processor HTTP API. */
    private String baseUrl = "http://localhost:8090";

    /** Default window size in seconds for client-side queries. */
    private int windowSeconds = 60;

    public String getBaseUrl() { return baseUrl; }
    public void setBaseUrl(String baseUrl) { this.baseUrl = baseUrl; }

    public int getWindowSeconds() { return windowSeconds; }
    public void setWindowSeconds(int windowSeconds) { this.windowSeconds = windowSeconds; }
}
