package dev.pushkar.ws;

import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.TextWebSocketHandler;

import java.io.IOException;
import java.net.URI;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

/**
 * Spring WebSocket client connecting to the Go WebSocket pub/sub gateway.
 *
 * <p>Uses {@link StandardWebSocketClient} (backed by the JDK HTTP client's
 * WebSocket implementation in Java 11+) — no additional WebSocket library needed.
 *
 * <p>This class is intentionally kept under 55 lines of functional code to show
 * the minimal surface area required for the pub/sub join-and-receive pattern.
 */
public class WsGatewayClient extends TextWebSocketHandler {

    private final String gatewayUrl;
    private final String userID;
    private WebSocketSession session;

    private final List<String> received = new CopyOnWriteArrayList<>();
    private final CountDownLatch connected = new CountDownLatch(1);

    public WsGatewayClient(String gatewayUrl, String userID) {
        this.gatewayUrl = gatewayUrl;
        this.userID = userID;
    }

    /** Connect and return once the WebSocket handshake completes. */
    public void connect() throws Exception {
        var wsClient = new StandardWebSocketClient();
        wsClient.execute(this, gatewayUrl + "?user=" + userID);
        if (!connected.await(5, TimeUnit.SECONDS)) {
            throw new IllegalStateException("WebSocket connect timeout for user=" + userID);
        }
    }

    /** Join a named pub/sub room. */
    public void joinRoom(String room) throws IOException {
        send("{\"action\":\"join\",\"room\":\"" + room + "\"}");
    }

    /** Broadcast a text message to the current room. */
    public void sendMessage(String content) throws IOException {
        send("{\"action\":\"message\",\"content\":\"" + content + "\"}");
    }

    /** Returns a snapshot of all messages received since connect(). */
    public List<String> getReceived() { return new ArrayList<>(received); }

    /** Gracefully close the WebSocket session. */
    public void close() throws IOException {
        if (session != null && session.isOpen()) session.close();
    }

    @Override
    public void afterConnectionEstablished(WebSocketSession s) {
        this.session = s;
        connected.countDown();
    }

    @Override
    protected void handleTextMessage(WebSocketSession s, TextMessage message) {
        received.add(message.getPayload());
    }

    private void send(String json) throws IOException {
        if (session == null || !session.isOpen()) throw new IOException("not connected");
        session.sendMessage(new TextMessage(json));
    }
}
