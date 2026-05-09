package dev.pushkar.httpserver;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the HTTP lab server connection.
 * Bound from the "http-lab" prefix in application.yml.
 *
 * <pre>
 * http-lab:
 *   base-url: http://localhost:8080
 *   connection-pool:
 *     max-connections: 10
 * </pre>
 */
@ConfigurationProperties("http-lab")
public class HttpServerProperties {

    /** Base URL of the Go HTTP server (no trailing slash). */
    private String baseUrl = "http://localhost:8080";

    private ConnectionPool connectionPool = new ConnectionPool();

    public String getBaseUrl() {
        return baseUrl;
    }

    public void setBaseUrl(String baseUrl) {
        this.baseUrl = baseUrl;
    }

    public ConnectionPool getConnectionPool() {
        return connectionPool;
    }

    public void setConnectionPool(ConnectionPool connectionPool) {
        this.connectionPool = connectionPool;
    }

    /** Connection pool settings forwarded to Reactor Netty. */
    public static class ConnectionPool {

        /** Maximum number of connections in the pool (default: 10). */
        private int maxConnections = 10;

        public int getMaxConnections() {
            return maxConnections;
        }

        public void setMaxConnections(int maxConnections) {
            this.maxConnections = maxConnections;
        }
    }
}
