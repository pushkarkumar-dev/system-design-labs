package dev.pushkar.mcp;

import org.springframework.boot.context.properties.ConfigurationProperties;

/**
 * Configuration properties for the MCP client.
 *
 * Set these in application.yml:
 *   mcp:
 *     server-command: python
 *     server-args: -m src.example_server
 *     server-url: http://localhost:8080   # for HTTP transport
 *     transport: stdio                     # stdio or http
 */
@ConfigurationProperties(prefix = "mcp")
public class McpProperties {

    /** Python executable (default: python3) */
    private String serverCommand = "python3";

    /** Arguments to the server command (default: -m src.example_server) */
    private String serverArgs = "-m src.example_server";

    /** HTTP server URL (for HTTP/SSE transport) */
    private String serverUrl = "http://localhost:8080";

    /** Transport type: "stdio" (subprocess) or "http" (remote server) */
    private String transport = "stdio";

    /** Working directory for the subprocess (default: labs/mcp-server) */
    private String workingDir = "labs/mcp-server";

    public String getServerCommand() { return serverCommand; }
    public void setServerCommand(String serverCommand) { this.serverCommand = serverCommand; }

    public String getServerArgs() { return serverArgs; }
    public void setServerArgs(String serverArgs) { this.serverArgs = serverArgs; }

    public String getServerUrl() { return serverUrl; }
    public void setServerUrl(String serverUrl) { this.serverUrl = serverUrl; }

    public String getTransport() { return transport; }
    public void setTransport(String transport) { this.transport = transport; }

    public String getWorkingDir() { return workingDir; }
    public void setWorkingDir(String workingDir) { this.workingDir = workingDir; }
}
