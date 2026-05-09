package dev.pushkar.tokenizer;

import org.springframework.web.client.RestClient;
import org.springframework.http.MediaType;
import java.util.List;

/**
 * HTTP client that talks to the Python FastAPI tokenizer server.
 *
 * The server exposes:
 *   POST /encode  {text: str}          -> {tokens: List[int], token_strings: List[str]}
 *   POST /decode  {tokens: List[int]}  -> {text: str}
 *   GET  /health                       -> {status: str, vocab_size: int}
 *   GET  /vocab_size                   -> {vocab_size: int}
 *
 * Built on Spring Framework 6.1's RestClient — the modern fluent replacement
 * for RestTemplate (which is in maintenance mode since Spring 5.0).
 */
public class TokenizerClient {

    private final RestClient http;

    public TokenizerClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .defaultHeader("Accept", MediaType.APPLICATION_JSON_VALUE)
                .build();
    }

    // ------------------------------------------------------------------
    // Core operations
    // ------------------------------------------------------------------

    /**
     * Tokenize text into integer IDs.
     *
     * @param text  Input string to tokenize.
     * @return      TokenizeResult containing token IDs and their string forms.
     */
    public TokenizeResult encode(String text) {
        var request = new EncodeRequest(text);
        var response = http.post()
                .uri("/encode")
                .contentType(MediaType.APPLICATION_JSON)
                .body(request)
                .retrieve()
                .body(EncodeResponse.class);
        if (response == null) {
            throw new TokenizerException("Null response from /encode");
        }
        return new TokenizeResult(response.tokens(), response.token_strings());
    }

    /**
     * Reconstruct the original string from token IDs.
     *
     * @param tokens  List of integer token IDs (from a previous encode() call).
     * @return        The decoded string.
     */
    public String decode(List<Integer> tokens) {
        var request = new DecodeRequest(tokens);
        var response = http.post()
                .uri("/decode")
                .contentType(MediaType.APPLICATION_JSON)
                .body(request)
                .retrieve()
                .body(DecodeResponse.class);
        if (response == null) {
            throw new TokenizerException("Null response from /decode");
        }
        return response.text();
    }

    /**
     * Health check — also returns the vocabulary size.
     */
    public HealthResponse health() {
        var response = http.get()
                .uri("/health")
                .retrieve()
                .body(HealthResponse.class);
        if (response == null) {
            throw new TokenizerException("Null response from /health");
        }
        return response;
    }

    /**
     * Returns the vocabulary size without a full health check.
     */
    public int vocabSize() {
        var response = http.get()
                .uri("/vocab_size")
                .retrieve()
                .body(VocabSizeResponse.class);
        if (response == null) {
            throw new TokenizerException("Null response from /vocab_size");
        }
        return response.vocab_size();
    }

    // ------------------------------------------------------------------
    // DTOs — Java 16 records for immutable, zero-boilerplate data
    // ------------------------------------------------------------------

    /** Result of an encode() call. */
    public record TokenizeResult(
            List<Integer> tokens,
            List<String> tokenStrings
    ) {}

    // --- Request bodies ---
    record EncodeRequest(String text) {}
    record DecodeRequest(List<Integer> tokens) {}

    // --- Response bodies (field names match JSON snake_case from Python) ---
    record EncodeResponse(List<Integer> tokens, List<String> token_strings) {}
    record DecodeResponse(String text) {}
    public record HealthResponse(String status, int vocab_size) {}
    record VocabSizeResponse(int vocab_size) {}

    // --- Exception ---
    public static class TokenizerException extends RuntimeException {
        public TokenizerException(String message) { super(message); }
        public TokenizerException(String message, Throwable cause) { super(message, cause); }
    }
}
