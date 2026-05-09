package dev.pushkar.lock;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the distributed lock client.
 *
 * <p>Example {@code application.yml}:
 * <pre>{@code
 * dist-lock:
 *   service-url: http://localhost:8080
 *   default-ttl-ms: 5000
 *   retry-attempts: 3
 *   retry-delay-ms: 100
 * }</pre>
 */
@ConfigurationProperties(prefix = "dist-lock")
public class LockProperties {

    /** Base URL of the Go lock server. */
    private String serviceUrl = "http://localhost:8080";

    /** Default lock TTL in milliseconds (used when the annotation does not specify ttlMs). */
    private long defaultTtlMs = 5_000L;

    /** Maximum number of acquire attempts before throwing {@link IllegalStateException}. */
    private int retryAttempts = 3;

    /** Delay between retry attempts in milliseconds. */
    private long retryDelayMs = 100L;

    public String serviceUrl() { return serviceUrl; }
    public long defaultTtlMs() { return defaultTtlMs; }
    public int retryAttempts() { return retryAttempts; }
    public long retryDelayMs() { return retryDelayMs; }

    public void setServiceUrl(String serviceUrl) { this.serviceUrl = serviceUrl; }
    public void setDefaultTtlMs(long defaultTtlMs) { this.defaultTtlMs = defaultTtlMs; }
    public void setRetryAttempts(int retryAttempts) { this.retryAttempts = retryAttempts; }
    public void setRetryDelayMs(long retryDelayMs) { this.retryDelayMs = retryDelayMs; }
}
