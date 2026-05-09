package com.labs.voice;

import org.springframework.boot.CommandLineRunner;
import org.springframework.stereotype.Component;
import org.springframework.web.socket.BinaryMessage;
import org.springframework.web.socket.TextMessage;
import org.springframework.web.socket.WebSocketSession;
import org.springframework.web.socket.client.standard.StandardWebSocketClient;
import org.springframework.web.socket.handler.AbstractWebSocketHandler;

import java.io.ByteArrayOutputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.logging.Logger;

/**
 * Connects to the Python voice assistant server over WebSocket, streams
 * binary 16kHz mono 16-bit PCM audio in 10ms chunks, and collects
 * WAV audio response chunks.
 *
 * <p>Binary WebSocket protocol:
 * <ul>
 *   <li>Client → Server: binary frames of exactly 320 bytes (160 samples × 2 bytes)</li>
 *   <li>Server → Client: binary frames (WAV audio response chunks)</li>
 *   <li>Server → Client: text frame JSON {"transcript": "...", "response": "..."}</li>
 * </ul>
 * </p>
 */
@Component
public class VoiceWebSocketClient implements CommandLineRunner {

    private static final Logger log = Logger.getLogger(VoiceWebSocketClient.class.getName());

    /** 160 samples × 2 bytes per sample = one 10ms chunk of 16kHz mono 16-bit PCM */
    private static final int CHUNK_BYTES = 320;

    /** Milliseconds to sleep between chunk sends (simulates real-time audio capture) */
    private static final int CHUNK_INTERVAL_MS = 10;

    private final VoiceProperties props;

    public VoiceWebSocketClient(VoiceProperties props) {
        this.props = props;
    }

    @Override
    public void run(String... args) throws Exception {
        System.out.println("\n=== Voice Assistant — Spring WebSocket Demo ===\n");
        System.out.println("Server URL:   " + props.wsUrl());
        System.out.println("Test audio:   " + props.testAudioPath());
        System.out.println("Output file:  " + props.outputPath());
        System.out.println();

        // Load test audio from classpath
        byte[] pcmBytes = loadTestAudio();
        if (pcmBytes == null) {
            System.out.println("[DEMO MODE] No test audio found on classpath.");
            System.out.println("            Creating synthetic 1-second 440Hz sine wave as test audio.");
            pcmBytes = generateSineWave(440, 1000, 0.3f);
        }

        System.out.printf("Audio loaded: %d bytes = %.1f ms of audio%n",
                pcmBytes.length, pcmBytes.length / 32.0);

        // Collect response data
        List<byte[]> audioResponseChunks = new ArrayList<>();
        String[] transcriptHolder = {null};
        CountDownLatch responseLatch = new CountDownLatch(1);

        // Build WebSocket handler
        AbstractWebSocketHandler handler = new AbstractWebSocketHandler() {

            @Override
            public void afterConnectionEstablished(WebSocketSession session) {
                log.info("Connected to " + session.getUri());
            }

            @Override
            protected void handleBinaryMessage(WebSocketSession session, BinaryMessage message) {
                byte[] chunk = new byte[message.getPayload().remaining()];
                message.getPayload().get(chunk);
                audioResponseChunks.add(chunk);
                System.out.printf("  [audio chunk] received %d bytes of WAV audio%n", chunk.length);
            }

            @Override
            protected void handleTextMessage(WebSocketSession session, TextMessage message) {
                transcriptHolder[0] = message.getPayload();
                System.out.println("  [text frame]  " + message.getPayload());
                responseLatch.countDown();
            }
        };

        // Connect to the voice assistant WebSocket server
        StandardWebSocketClient wsClient = new StandardWebSocketClient();
        WebSocketSession session = wsClient
                .execute(handler, props.wsUrl())
                .get(10, TimeUnit.SECONDS);

        System.out.println("Connected. Streaming " + pcmBytes.length / CHUNK_BYTES + " audio chunks...\n");

        // Stream audio in 10ms chunks
        int totalChunks = pcmBytes.length / CHUNK_BYTES;
        for (int i = 0; i < totalChunks; i++) {
            byte[] chunk = new byte[CHUNK_BYTES];
            System.arraycopy(pcmBytes, i * CHUNK_BYTES, chunk, 0, CHUNK_BYTES);

            ByteBuffer buffer = ByteBuffer.wrap(chunk);
            session.sendMessage(new BinaryMessage(buffer));

            // Simulate real-time audio rate: one chunk every 10ms
            Thread.sleep(CHUNK_INTERVAL_MS);
        }

        System.out.println("\nAll audio chunks sent. Waiting for response...");

        // Wait up to 30 seconds for the server to respond
        boolean received = responseLatch.await(30, TimeUnit.SECONDS);

        session.close();

        if (!received) {
            System.out.println("\n[TIMEOUT] No response received within 30s.");
            System.out.println("          Is the Python server running? (uvicorn src.server:app --port 8000)");
            return;
        }

        // Save concatenated audio response to output file
        if (!audioResponseChunks.isEmpty()) {
            byte[] combined = combineWavChunks(audioResponseChunks);
            try (FileOutputStream fos = new FileOutputStream(props.outputPath())) {
                fos.write(combined);
            }
            System.out.printf("%nResponse audio: %d chunks, %d bytes total → saved to %s%n",
                    audioResponseChunks.size(), combined.length, props.outputPath());
        }

        System.out.println("\nDone.");
    }

    /**
     * Load test audio PCM from classpath resource, stripping the WAV header.
     * Returns null if the resource is not found.
     */
    private byte[] loadTestAudio() {
        try (InputStream is = getClass().getClassLoader().getResourceAsStream(props.testAudioPath())) {
            if (is == null) return null;

            byte[] wavBytes = is.readAllBytes();

            // Skip 44-byte WAV header to get raw PCM
            if (wavBytes.length > 44 && wavBytes[0] == 'R' && wavBytes[1] == 'I') {
                byte[] pcm = new byte[wavBytes.length - 44];
                System.arraycopy(wavBytes, 44, pcm, 0, pcm.length);
                return pcm;
            }
            return wavBytes;
        } catch (IOException e) {
            return null;
        }
    }

    /**
     * Generate a sine wave as raw 16-bit PCM bytes (16kHz mono).
     *
     * @param frequencyHz Frequency in Hz (440 = A4)
     * @param durationMs  Duration in milliseconds
     * @param amplitude   Amplitude in [0.0, 1.0]
     */
    private byte[] generateSineWave(int frequencyHz, int durationMs, float amplitude) {
        int sampleRate = 16_000;
        int nSamples = sampleRate * durationMs / 1000;
        byte[] pcm = new byte[nSamples * 2]; // 2 bytes per 16-bit sample

        for (int i = 0; i < nSamples; i++) {
            double t = (double) i / sampleRate;
            short value = (short) (amplitude * 32767 * Math.sin(2 * Math.PI * frequencyHz * t));
            // Little-endian 16-bit
            pcm[i * 2]     = (byte) (value & 0xFF);
            pcm[i * 2 + 1] = (byte) ((value >> 8) & 0xFF);
        }
        return pcm;
    }

    /**
     * Combine multiple WAV byte arrays by concatenating their PCM frames.
     * Assumes all chunks share the same format (16kHz, mono, 16-bit).
     * Returns the first chunk as-is if there is only one.
     */
    private byte[] combineWavChunks(List<byte[]> chunks) {
        if (chunks.size() == 1) return chunks.get(0);

        // Simple approach: return the first chunk (which is a complete WAV)
        // For production use, parse RIFF headers and concatenate PCM data
        ByteArrayOutputStream out = new ByteArrayOutputStream();
        for (byte[] chunk : chunks) {
            try {
                // Skip 44-byte header for chunks after the first
                out.write(chunk);
            } catch (IOException ignored) {}
        }
        return out.toByteArray();
    }
}
