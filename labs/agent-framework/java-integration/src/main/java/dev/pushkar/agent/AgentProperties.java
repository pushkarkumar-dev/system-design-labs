package dev.pushkar.agent;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the agent framework integration.
 *
 * <p>Set in {@code application.yml} under the {@code agent} prefix:
 * <pre>
 * agent:
 *   base-url: http://localhost:8001
 *   default-mode: function
 * </pre>
 */
@ConfigurationProperties(prefix = "agent")
public class AgentProperties {

    /** Base URL of the Python agent framework server. */
    private String baseUrl = "http://localhost:8001";

    /** Default agent mode: 'react' or 'function'. */
    private String defaultMode = "function";

    public String getBaseUrl() { return baseUrl; }
    public void setBaseUrl(String baseUrl) { this.baseUrl = baseUrl; }

    public String getDefaultMode() { return defaultMode; }
    public void setDefaultMode(String defaultMode) { this.defaultMode = defaultMode; }
}
