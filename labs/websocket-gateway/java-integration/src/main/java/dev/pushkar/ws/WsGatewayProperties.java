package dev.pushkar.ws;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the WebSocket gateway client.
 *
 * <p>Set in {@code application.properties}:
 * <pre>
 *   ws-gateway.url=ws://localhost:8080/ws
 *   ws-gateway.connect-timeout-ms=5000
 * </pre>
 */
@ConfigurationProperties(prefix = "ws-gateway")
public class WsGatewayProperties {

    /** WebSocket URL of the Go gateway. */
    private String url = "ws://localhost:8080/ws";

    /** Connection timeout in milliseconds. */
    private int connectTimeoutMs = 5000;

    public String getUrl() { return url; }
    public void setUrl(String url) { this.url = url; }

    public int getConnectTimeoutMs() { return connectTimeoutMs; }
    public void setConnectTimeoutMs(int connectTimeoutMs) { this.connectTimeoutMs = connectTimeoutMs; }
}
