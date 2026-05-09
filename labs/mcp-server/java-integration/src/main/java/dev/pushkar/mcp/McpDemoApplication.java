package dev.pushkar.mcp;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.mcp.spec.McpSchema;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.List;
import java.util.Map;

/**
 * MCP Demo Application.
 *
 * Demonstrates the full MCP client lifecycle:
 *   1. McpAutoConfiguration spawns our Python MCP server (subprocess via stdio)
 *   2. McpSyncClient runs the initialize handshake automatically
 *   3. We list tools, resources, prompts from the Python server
 *   4. We call each tool and read a resource
 *   5. The subprocess is cleaned up when the Spring context closes
 *
 * Expected output (with Python MCP server running):
 *
 *   === MCP Server Demo ===
 *
 *   Available tools (3):
 *     - read_file: Read the text contents of a file.
 *     - list_files: List files in a directory.
 *     - execute_python: Execute a Python expression and return the result.
 *
 *   Calling execute_python(code="len('hello world')"):
 *     Result: 11
 *
 *   Calling list_files(directory="."):
 *     Result: ["file: pyproject.toml", "dir: src", "dir: tests", ...]
 *
 *   Reading resource: system://info
 *     platform: Darwin-...
 *     python_version: 3.12.x
 *
 *   Rendered prompt 'code_review':
 *     Please review the following Python code...
 *
 *   Done. MCP client closed.
 */
@SpringBootApplication
public class McpDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(McpDemoApplication.class);

    public static void main(String[] args) {
        SpringApplication.run(McpDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(McpClient client) {
        return args -> {
            System.out.println("=== MCP Server Demo ===\n");

            // 1. List all tools
            List<McpSchema.Tool> tools = client.listTools();
            System.out.printf("Available tools (%d):%n", tools.size());
            for (McpSchema.Tool tool : tools) {
                System.out.printf("  - %s: %s%n", tool.name(), tool.description());
            }
            System.out.println();

            // 2. Call execute_python tool
            System.out.println("Calling execute_python(code=\"len('hello world')\"):");
            String pyResult = client.callTool("execute_python", Map.of("code", "len('hello world')"));
            System.out.printf("  Result: %s%n%n", pyResult);

            // 3. Call list_files tool
            System.out.println("Calling list_files(directory=\".\"):");
            String filesResult = client.callTool("list_files", Map.of("directory", "."));
            // Print first 3 entries only
            String[] lines = filesResult.split("\n");
            for (int i = 0; i < Math.min(3, lines.length); i++) {
                System.out.printf("  %s%n", lines[i].trim());
            }
            if (lines.length > 3) {
                System.out.printf("  ... (%d more entries)%n", lines.length - 3);
            }
            System.out.println();

            // 4. Read a resource
            System.out.println("Reading resource: system://info");
            String sysInfo = client.readResource("system://info");
            // Print just the platform and python_version fields
            for (String line : sysInfo.split("\n")) {
                if (line.contains("platform") || line.contains("python_version")) {
                    System.out.printf("  %s%n", line.trim());
                }
            }
            System.out.println();

            // 5. List and render a prompt
            List<McpSchema.Prompt> prompts = client.listPrompts();
            if (!prompts.isEmpty()) {
                String promptName = prompts.get(0).name();
                System.out.printf("Rendered prompt '%s':%n", promptName);
                String rendered = client.getPrompt(promptName, Map.of(
                        "language", "Python",
                        "code", "x = [i for i in range(10) if i % 2 == 0]"
                ));
                // Print first 80 chars
                System.out.printf("  %s...%n%n", rendered.substring(0, Math.min(80, rendered.length())));
            }

            System.out.println("Done. MCP client closed.");
            client.close();
        };
    }
}
