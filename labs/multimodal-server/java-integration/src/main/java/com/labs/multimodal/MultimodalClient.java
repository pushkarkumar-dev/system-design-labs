package com.labs.multimodal;

import org.springframework.http.MediaType;
import org.springframework.web.client.RestClient;

import java.util.Base64;
import java.util.List;
import java.util.Map;

/**
 * Spring RestClient wrapper for the Python multimodal stub server.
 *
 * <p>Builds an OpenAI vision-format request with text and a base64-encoded image,
 * posts it to {@code POST /v1/chat/completions}, and returns the content string
 * from {@code choices[0].message.content}.
 *
 * <p>The OpenAI vision API format sends images as base64-encoded data URIs inside
 * a JSON body — no multipart upload required. This means any HTTP client that can
 * POST JSON (RestClient, OkHttp, Feign, Spring AI) works without special handling.
 *
 * <p>Why RestClient over RestTemplate?
 * RestTemplate is in maintenance mode since Spring 5.0. RestClient (Spring 6.1+)
 * is fluent, type-inferred, and throws RestClientException on non-2xx responses
 * automatically. The learning: APIs that encode the happy path in the type system
 * ({@code .retrieve().body(T.class)}) reduce error-handling boilerplate.
 */
public class MultimodalClient {

    private final RestClient http;

    public MultimodalClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Content-Type", MediaType.APPLICATION_JSON_VALUE)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    /**
     * Send a vision request: text prompt + base64-encoded image bytes.
     *
     * @param prompt     The text question (e.g. "What is in this image?")
     * @param imageBytes Raw image bytes (PNG, JPEG, etc.)
     * @param mimeType   MIME type string, e.g. "image/png"
     * @param maxTokens  Maximum tokens to generate
     * @return           Generated response string from the model
     */
    public String chat(String prompt, byte[] imageBytes, String mimeType, int maxTokens) {
        String b64 = Base64.getEncoder().encodeToString(imageBytes);
        String dataUri = "data:" + mimeType + ";base64," + b64;

        // Build OpenAI vision-format request body
        var requestBody = Map.of(
                "model", "multimodal-stub-v1",
                "max_tokens", maxTokens,
                "messages", List.of(
                        Map.of(
                                "role", "user",
                                "content", List.of(
                                        Map.of("type", "text", "text", prompt),
                                        Map.of("type", "image_url",
                                               "image_url", Map.of("url", dataUri))
                                )
                        )
                )
        );

        @SuppressWarnings("unchecked")
        var response = http.post()
                .uri("/v1/chat/completions")
                .body(requestBody)
                .retrieve()
                .body(Map.class);

        if (response == null) {
            throw new RuntimeException("Null response from multimodal server");
        }

        // Extract choices[0].message.content
        @SuppressWarnings("unchecked")
        var choices = (List<Map<String, Object>>) response.get("choices");
        if (choices == null || choices.isEmpty()) {
            throw new RuntimeException("No choices in response: " + response);
        }
        @SuppressWarnings("unchecked")
        var message = (Map<String, Object>) choices.get(0).get("message");
        return (String) message.get("content");
    }

    /**
     * Text-only request (no image).
     */
    public String chat(String prompt, int maxTokens) {
        var requestBody = Map.of(
                "model", "multimodal-stub-v1",
                "max_tokens", maxTokens,
                "messages", List.of(
                        Map.of("role", "user", "content", prompt)
                )
        );

        @SuppressWarnings("unchecked")
        var response = http.post()
                .uri("/v1/chat/completions")
                .body(requestBody)
                .retrieve()
                .body(Map.class);

        if (response == null) throw new RuntimeException("Null response");

        @SuppressWarnings("unchecked")
        var choices = (List<Map<String, Object>>) response.get("choices");
        @SuppressWarnings("unchecked")
        var message = (Map<String, Object>) choices.get(0).get("message");
        return (String) message.get("content");
    }

    /** Health check — returns true if the server responds with status=ok. */
    public boolean isHealthy() {
        try {
            @SuppressWarnings("unchecked")
            var result = http.get().uri("/health").retrieve().body(Map.class);
            return result != null && "ok".equals(result.get("status"));
        } catch (Exception e) {
            return false;
        }
    }
}
