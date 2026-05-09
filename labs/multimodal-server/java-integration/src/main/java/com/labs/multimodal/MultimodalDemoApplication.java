package com.labs.multimodal;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.CommandLineRunner;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;

/**
 * Spring Boot demo application for the multimodal stub server.
 *
 * <p>Reads a test image from the classpath, base64-encodes it, and sends it
 * to the Python multimodal server along with a text prompt. Requires the
 * Python server to be running on localhost:8000.
 *
 * <p>Run:
 * <pre>
 *   # Terminal 1: start the Python server
 *   cd labs/multimodal-server
 *   uvicorn src.v2_server:app --port 8000
 *
 *   # Terminal 2: run this Spring Boot demo
 *   cd labs/multimodal-server/java-integration
 *   mvn spring-boot:run
 * </pre>
 */
@SpringBootApplication
public class MultimodalDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(MultimodalDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo(
            @Value("${multimodal.base-url:http://localhost:8000}") String baseUrl
    ) {
        return args -> {
            var client = new MultimodalClient(baseUrl);

            System.out.println("=== Multimodal Server Spring Integration Demo ===");
            System.out.println("Server: " + baseUrl);

            // Health check
            boolean healthy = client.isHealthy();
            System.out.println("Health: " + (healthy ? "UP" : "DOWN"));

            if (!healthy) {
                System.out.println(
                    "Server not reachable. Start it with:\n" +
                    "  cd labs/multimodal-server && uvicorn src.v2_server:app --port 8000"
                );
                return;
            }

            // Load a test image from classpath (test-image.png)
            byte[] imageBytes = loadClasspathResource("test-image.png");
            if (imageBytes == null) {
                System.out.println("No test-image.png found on classpath — using text-only mode.");
                String textResponse = client.chat("What is a vision-language model?", 50);
                System.out.println("\nText-only response:");
                System.out.println("  " + textResponse);
                return;
            }

            // Send multimodal request: image + text
            System.out.println("\n--- Vision request: image + text ---");
            String prompt = "What is in this image?";
            System.out.println("Prompt: " + prompt);
            String response = client.chat(prompt, imageBytes, "image/png", 50);
            System.out.println("Response: " + response);

            // Demonstrate that any Spring AI client would work the same way:
            // Spring AI's ChatClient with Media type calls the same API contract.
            // Replace MultimodalClient with:
            //   ChatClient.create(openAiChatModel)
            //       .prompt()
            //       .user(u -> u.text(prompt).media(MimeTypeUtils.IMAGE_PNG, imageResource))
            //       .call()
            //       .content()
            // Both produce a String response — the API contract is identical.
            System.out.println(
                "\nNote: Spring AI's ChatClient with Media type sends the same OpenAI" +
                " vision format. Replace MultimodalClient with Spring AI for production use."
            );

            System.out.println("\nDone.");
        };
    }

    private static byte[] loadClasspathResource(String name) {
        try (InputStream is = MultimodalDemoApplication.class
                .getClassLoader().getResourceAsStream(name)) {
            if (is == null) return null;
            var buf = new ByteArrayOutputStream();
            byte[] chunk = new byte[8192];
            int n;
            while ((n = is.read(chunk)) != -1) buf.write(chunk, 0, n);
            return buf.toByteArray();
        } catch (Exception e) {
            return null;
        }
    }
}
