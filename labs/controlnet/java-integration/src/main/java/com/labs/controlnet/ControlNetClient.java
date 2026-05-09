package com.labs.controlnet;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;
import org.springframework.web.client.RestClient;

import java.util.Base64;
import java.util.List;
import java.util.Map;

/**
 * HTTP client for the Python ControlNet generation server.
 *
 * <p>Wraps POST /generate with base64-encoded control image.
 * The server runs on localhost:8000 by default.
 *
 * <p>Usage:
 * <pre>
 *   byte[] imageBytes = Files.readAllBytes(Path.of("edge.png"));
 *   byte[] generated = client.generate(imageBytes, "canny", 1.0f, 20);
 * </pre>
 *
 * <p>The returned bytes are a PNG-encoded image at 64x64 resolution.
 */
@Component
public class ControlNetClient {

    private final RestClient http;

    public ControlNetClient(
            @Value("${controlnet.base-url:http://localhost:8000}") String baseUrl
    ) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Content-Type", "application/json")
                .build();
    }

    /**
     * Call POST /generate with a control image.
     *
     * @param controlImageBytes raw bytes of a PNG or JPEG control image
     * @param mode              preprocessor mode: "canny", "depth", or "pose"
     * @param scale             conditioning scale in [0.0, 2.0]
     * @param steps             number of DDIM sampling steps
     * @return PNG-encoded bytes of the generated image
     */
    public byte[] generate(byte[] controlImageBytes, String mode, float scale, int steps) {
        String encoded = Base64.getEncoder().encodeToString(controlImageBytes);

        Map<String, Object> body = Map.of(
                "control_image", encoded,
                "mode", mode,
                "scale", scale,
                "steps", steps,
                "seed", 42,
                "prompt_embedding", List.of()
        );

        @SuppressWarnings("unchecked")
        Map<String, Object> response = http.post()
                .uri("/generate")
                .body(body)
                .retrieve()
                .body(Map.class);

        if (response == null || !response.containsKey("generated_image")) {
            throw new RuntimeException("Invalid response from ControlNet server");
        }

        String generatedBase64 = (String) response.get("generated_image");
        return Base64.getDecoder().decode(generatedBase64);
    }

    /**
     * Call GET /modes to list available preprocessor modes.
     *
     * @return list of mode names, e.g. ["canny", "depth", "pose"]
     */
    @SuppressWarnings("unchecked")
    public List<String> getModes() {
        return http.get()
                .uri("/modes")
                .retrieve()
                .body(List.class);
    }

    /**
     * Call GET /health to verify the server is running.
     *
     * @return true if the server responds with status "ok"
     */
    @SuppressWarnings("unchecked")
    public boolean isHealthy() {
        try {
            Map<String, Object> resp = http.get()
                    .uri("/health")
                    .retrieve()
                    .body(Map.class);
            return resp != null && "ok".equals(resp.get("status"));
        } catch (Exception e) {
            return false;
        }
    }
}
