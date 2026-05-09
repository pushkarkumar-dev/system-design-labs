package dev.pushkar.flags;

import org.springframework.core.ParameterizedTypeReference;
import org.springframework.web.client.RestClient;
import org.springframework.web.client.RestClientException;

import java.util.List;
import java.util.Map;

/**
 * Thin HTTP client for the Go feature flag server.
 *
 * <p>Wraps three routes:
 * <ul>
 *   <li>{@link #evaluate} — evaluate a named flag for a user context
 *   <li>{@link #listFlags} — retrieve all flags
 *   <li>{@link #updateFlag} — create or update a flag configuration
 * </ul>
 *
 * <p>Uses Spring Framework 6.1's {@link RestClient} — the fluent, type-safe
 * replacement for the deprecated {@code RestTemplate}. Non-2xx responses throw
 * {@link RestClientException} automatically.
 *
 * <p>Hard cap: this class is kept under 60 lines of non-comment code.
 * Retry, circuit-breaking, and reactive variants belong in {@link FlagCache}.
 */
public class FlagClient {

    private final RestClient http;

    public FlagClient(String baseUrl) {
        this.http = RestClient.builder()
                .baseUrl(baseUrl)
                .build();
    }

    /**
     * Evaluate a flag for the given user context.
     *
     * @return {@code true} if the flag is enabled for this user
     */
    public boolean evaluate(String flagName, String userId, String email) {
        String uri = "/flags/{name}/evaluate?user_id={uid}&email={email}";
        var resp = http.get()
                .uri(uri, flagName, userId != null ? userId : "", email != null ? email : "")
                .retrieve()
                .body(EvaluateResponse.class);
        return resp != null && resp.enabled();
    }

    /** Returns all flags currently configured in the service. */
    public List<FlagInfo> listFlags() {
        var result = http.get()
                .uri("/flags")
                .retrieve()
                .body(new ParameterizedTypeReference<List<FlagInfo>>() {});
        return result != null ? result : List.of();
    }

    /**
     * Create or replace a flag by name.
     *
     * @param name    flag identifier
     * @param enabled the new default_enabled state
     */
    public void updateFlag(String name, boolean enabled) {
        http.put()
                .uri("/flags/{name}", name)
                .body(Map.of("name", name, "default_enabled", enabled))
                .retrieve()
                .toBodilessEntity();
    }

    // ── Response record types ────────────────────────────────────────────────

    record EvaluateResponse(String flag, boolean enabled, boolean exists) {}

    /** Lightweight representation of a flag as returned by GET /flags. */
    public record FlagInfo(String name, boolean default_enabled, String description) {}

    /** Thrown when a flag service call fails and no fallback is applicable. */
    public static class FlagServiceException extends RuntimeException {
        public FlagServiceException(String msg)              { super(msg); }
        public FlagServiceException(String msg, Throwable c) { super(msg, c); }
    }
}
