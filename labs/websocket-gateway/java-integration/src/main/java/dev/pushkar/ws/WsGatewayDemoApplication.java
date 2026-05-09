package dev.pushkar.ws;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;

/**
 * Demo: 5 Spring WebSocket clients connect to the Go gateway, join "test-room",
 * broadcast 100 messages from client-0, and verify all 5 clients receive them.
 *
 * <p>Run:
 * <pre>
 *   # Terminal 1: start the Go gateway
 *   cd labs/websocket-gateway
 *   go run ./cmd/server --port 8080
 *
 *   # Terminal 2: run this Spring Boot demo
 *   cd labs/websocket-gateway/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>Expected output (abridged):
 * <pre>
 *   === WebSocket Gateway Spring Integration Demo ===
 *   Connected 5 clients to ws://localhost:8080/ws
 *   All 5 clients joined room 'test-room'
 *   client-0 broadcast 100 messages
 *   Waiting for delivery...
 *   client-0 received: 102 messages (100 own + 2 presence events)
 *   client-1 received: 101 messages
 *   ...
 *   All clients received at least 100 messages. Test PASSED.
 * </pre>
 */
@SpringBootApplication
public class WsGatewayDemoApplication {

    private static final Logger log = LoggerFactory.getLogger(WsGatewayDemoApplication.class);
    private static final String GATEWAY_URL = "ws://localhost:8080/ws";
    private static final String ROOM = "test-room";
    private static final int NUM_CLIENTS = 5;
    private static final int NUM_MESSAGES = 100;

    public static void main(String[] args) {
        SpringApplication.run(WsGatewayDemoApplication.class, args);
    }

    @Bean
    public CommandLineRunner demo() {
        return args -> {
            log.info("=== WebSocket Gateway Spring Integration Demo ===");

            // ── Step 1: Connect 5 clients ────────────────────────────────────
            List<WsGatewayClient> clients = new ArrayList<>();
            for (int i = 0; i < NUM_CLIENTS; i++) {
                var client = new WsGatewayClient(GATEWAY_URL, "client-" + i);
                try {
                    client.connect();
                    clients.add(client);
                } catch (Exception e) {
                    log.error("Failed to connect client-{}: {}", i, e.getMessage());
                    log.error("Is the Go gateway running? Start it with: go run ./cmd/server");
                    System.exit(1);
                }
            }
            log.info("Connected {} clients to {}", NUM_CLIENTS, GATEWAY_URL);

            // ── Step 2: All clients join the same room ───────────────────────
            for (int i = 0; i < clients.size(); i++) {
                clients.get(i).joinRoom(ROOM);
            }
            log.info("All {} clients joined room '{}'", NUM_CLIENTS, ROOM);

            // Brief pause so presence events settle.
            TimeUnit.MILLISECONDS.sleep(200);

            // ── Step 3: client-0 broadcasts 100 messages ─────────────────────
            var sender = clients.get(0);
            for (int i = 0; i < NUM_MESSAGES; i++) {
                sender.sendMessage("msg-" + i);
            }
            log.info("client-0 broadcast {} messages", NUM_MESSAGES);

            // ── Step 4: Wait for delivery ────────────────────────────────────
            log.info("Waiting for delivery...");
            TimeUnit.MILLISECONDS.sleep(500);

            // ── Step 5: Verify all clients received the messages ─────────────
            boolean passed = true;
            for (int i = 0; i < clients.size(); i++) {
                int count = clients.get(i).getReceived().size();
                log.info("client-{} received: {} messages", i, count);
                // Each client receives 100 broadcast messages + presence events.
                // We conservatively check that at least 100 arrived.
                if (count < NUM_MESSAGES) {
                    log.error("client-{} only received {} messages, expected >= {}", i, count, NUM_MESSAGES);
                    passed = false;
                }
            }

            if (passed) {
                log.info("All clients received at least {} messages. Test PASSED.", NUM_MESSAGES);
            } else {
                log.error("Some clients missed messages. Test FAILED.");
            }

            // ── Step 6: Close all connections ────────────────────────────────
            for (var c : clients) {
                try { c.close(); } catch (Exception ignored) {}
            }
            log.info("All connections closed.");
        };
    }
}
