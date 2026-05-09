package dev.pushkar.faas;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Typed configuration for the FaaS client.
 *
 * Bind from application.yml:
 * <pre>
 * faas:
 *   base-url: http://localhost:8080
 *   default-timeout-seconds: 30
 * </pre>
 */
@ConfigurationProperties(prefix = "faas")
public class FaasProperties {

    /** Base URL of the FaaS runtime HTTP server. */
    private String baseUrl = "http://localhost:8080";

    /** Default invocation timeout in seconds. */
    private int defaultTimeoutSeconds = 30;

    public String getBaseUrl() { return baseUrl; }
    public void setBaseUrl(String baseUrl) { this.baseUrl = baseUrl; }

    public int getDefaultTimeoutSeconds() { return defaultTimeoutSeconds; }
    public void setDefaultTimeoutSeconds(int s) { this.defaultTimeoutSeconds = s; }
}
