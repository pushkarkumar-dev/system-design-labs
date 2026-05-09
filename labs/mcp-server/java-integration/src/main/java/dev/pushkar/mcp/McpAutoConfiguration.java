package dev.pushkar.mcp;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.mcp.client.McpSyncClient;
import org.springframework.ai.mcp.client.stdio.StdioClientTransport;
import org.springframework.ai.mcp.client.stdio.ServerParameters;
import org.springframework.boot.context.properties.EnableConfigurationProperties;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

import java.util.Arrays;

/**
 * Auto-configuration for the MCP client.
 *
 * Creates a McpSyncClient connected to our Python MCP server via stdio transport.
 * The Python process is spawned as a subprocess; its stdin/stdout become the
 * MCP transport channel.
 *
 * Wiring:
 *   McpProperties   (from application.yml: mcp.server-command, mcp.working-dir)
 *   McpSyncClient   (Spring AI — handles the MCP protocol + stdio transport)
 *   McpClient       (our wrapper — list tools, call tools, read resources)
 */
@Configuration
@EnableConfigurationProperties(McpProperties.class)
public class McpAutoConfiguration {

    private static final Logger log = LoggerFactory.getLogger(McpAutoConfiguration.class);

    /**
     * McpSyncClient configured with stdio transport pointing to our Python server.
     *
     * The ServerParameters builder sets:
     *   command:  python3 (or the configured serverCommand)
     *   args:     -m src.example_server (the Python MCP server module)
     *   workDir:  labs/mcp-server (relative to repo root)
     *
     * McpSyncClient.Builder.build() runs the initialize handshake automatically.
     * If the Python process fails to start, this bean creation throws an exception
     * and the application fails to start — fail-fast is the correct behavior.
     */
    @Bean(destroyMethod = "close")
    public McpSyncClient mcpSyncClient(McpProperties props) {
        log.info("Connecting to MCP server via stdio: {} {}", props.getServerCommand(), props.getServerArgs());

        String[] argsArray = props.getServerArgs().split("\\s+");

        ServerParameters serverParams = ServerParameters.builder(props.getServerCommand())
                .args(argsArray)
                .build();

        StdioClientTransport transport = new StdioClientTransport(serverParams);

        McpSyncClient client = McpSyncClient.builder()
                .transport(transport)
                .build();

        // Run the MCP initialize handshake
        client.initialize();

        log.info("MCP client initialized. Server: {}", client.getServerInfo().name());
        return client;
    }

    /** High-level McpClient wrapper bean. */
    @Bean
    public McpClient mcpClient(McpSyncClient syncClient) {
        return new McpClient(syncClient);
    }
}
