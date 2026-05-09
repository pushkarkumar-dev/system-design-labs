package dev.pushkar.mcp;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.mcp.client.McpSyncClient;
import org.springframework.ai.mcp.spec.McpSchema;

import java.util.List;
import java.util.Map;

/**
 * High-level MCP client wrapper (~60 lines).
 *
 * Wraps Spring AI's McpSyncClient with convenience methods for the operations
 * a typical Spring service needs: list tools, call a tool, list resources,
 * read a resource, list prompts, get a rendered prompt.
 *
 * The underlying McpSyncClient handles the MCP protocol: initialize handshake,
 * JSON-RPC message serialization, stdio/HTTP transport.
 *
 * Usage (see McpDemoApplication for a complete example):
 *   McpClient client = new McpClient(syncClient);
 *   List<McpSchema.Tool> tools = client.listTools();
 *   String result = client.callTool("execute_python", Map.of("code", "2+2"));
 */
public class McpClient {

    private static final Logger log = LoggerFactory.getLogger(McpClient.class);
    private final McpSyncClient delegate;

    public McpClient(McpSyncClient delegate) {
        this.delegate = delegate;
    }

    /** Return all tools the MCP server exposes. */
    public List<McpSchema.Tool> listTools() {
        McpSchema.ListToolsResult result = delegate.listTools();
        log.debug("MCP tools available: {}", result.tools().stream().map(McpSchema.Tool::name).toList());
        return result.tools();
    }

    /**
     * Call a tool and return its text output.
     * Returns the error message if isError=true.
     */
    public String callTool(String toolName, Map<String, Object> arguments) {
        McpSchema.CallToolResult result = delegate.callTool(
                new McpSchema.CallToolRequest(toolName, arguments)
        );
        String text = result.content().stream()
                .filter(c -> c instanceof McpSchema.TextContent)
                .map(c -> ((McpSchema.TextContent) c).text())
                .findFirst()
                .orElse("");
        if (result.isError()) {
            log.warn("MCP tool '{}' returned error: {}", toolName, text);
        }
        return text;
    }

    /** Return all resources the MCP server exposes. */
    public List<McpSchema.Resource> listResources() {
        return delegate.listResources().resources();
    }

    /** Read a resource by URI and return its text content. */
    public String readResource(String uri) {
        McpSchema.ReadResourceResult result = delegate.readResource(
                new McpSchema.ReadResourceRequest(uri)
        );
        return result.contents().stream()
                .filter(c -> c instanceof McpSchema.TextResourceContents)
                .map(c -> ((McpSchema.TextResourceContents) c).text())
                .findFirst()
                .orElse("");
    }

    /** Return all prompts the MCP server exposes. */
    public List<McpSchema.Prompt> listPrompts() {
        return delegate.listPrompts().prompts();
    }

    /** Get a rendered prompt with the given arguments. */
    public String getPrompt(String name, Map<String, String> arguments) {
        McpSchema.GetPromptResult result = delegate.getPrompt(
                new McpSchema.GetPromptRequest(name, arguments)
        );
        return result.messages().stream()
                .filter(m -> m.content() instanceof McpSchema.TextContent)
                .map(m -> ((McpSchema.TextContent) m.content()).text())
                .findFirst()
                .orElse("");
    }

    /** Close the underlying MCP client (closes the subprocess or HTTP connection). */
    public void close() {
        try {
            delegate.close();
        } catch (Exception e) {
            log.warn("Error closing MCP client: {}", e.getMessage());
        }
    }
}
