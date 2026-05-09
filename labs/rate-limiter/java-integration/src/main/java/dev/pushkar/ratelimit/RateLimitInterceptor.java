package dev.pushkar.ratelimit;

import jakarta.servlet.http.HttpServletRequest;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.web.client.RestClientException;
import org.springframework.web.servlet.HandlerInterceptor;

import java.time.Instant;
import java.time.temporal.ChronoUnit;

/**
 * Spring {@link HandlerInterceptor} that enforces rate limits on incoming requests.
 *
 * <h3>Lifecycle</h3>
 * <p>Spring calls interceptor methods in this order for each request:
 * <ol>
 *   <li>{@code preHandle} — before the controller runs; return false to short-circuit
 *   <li>{@code postHandle} — after the controller runs (if preHandle returned true)
 *   <li>{@code afterCompletion} — after the response is committed (always called)
 * </ol>
 *
 * <p>Rate limiting goes in {@code preHandle}: if the limit is exceeded, we set
 * 429 status + Retry-After header and return false, which stops the Spring
 * dispatcher from invoking the controller entirely.
 *
 * <h3>Key extraction</h3>
 * <p>The rate limit key is extracted from the {@code X-API-Key} header.
 * If not present, we fall back to the client's IP address (from
 * {@code X-Forwarded-For} or {@code getRemoteAddr}). This is the standard
 * approach for unauthenticated traffic.
 *
 * <h3>Fail-open behaviour</h3>
 * <p>If the rate limiter service is unreachable, the request is allowed through.
 * Failing closed (blocking all traffic when the rate limiter is down) is almost
 * always worse than allowing a brief overage.
 */
public class RateLimitInterceptor implements HandlerInterceptor {

    private static final Logger log = LoggerFactory.getLogger(RateLimitInterceptor.class);

    private final RateLimiterClient client;
    private final RateLimiterProperties props;

    public RateLimitInterceptor(RateLimiterClient client, RateLimiterProperties props) {
        this.client = client;
        this.props = props;
    }

    @Override
    public boolean preHandle(HttpServletRequest request,
                             HttpServletResponse response,
                             Object handler) throws Exception {
        if (!props.enabled()) {
            return true; // rate limiting disabled for local dev
        }

        String key = extractKey(request);
        String tier = request.getHeader("X-Tier");

        RateLimiterClient.RateLimitResult result;
        try {
            result = client.check(key, tier);
        } catch (RestClientException e) {
            // Rate limiter service unreachable — fail open
            log.warn("Rate limiter service unreachable ({}), failing open for key={}", e.getMessage(), key);
            return true;
        }

        if (!result.allowed()) {
            long retryAfterSeconds = retryAfter(result.resetAt());
            response.setStatus(429);
            response.setHeader("Retry-After", String.valueOf(retryAfterSeconds));
            response.setHeader("X-RateLimit-Remaining", "0");
            response.setContentType("application/json");
            response.getWriter().write(
                "{\"error\":\"rate limit exceeded\",\"retry_after_seconds\":" + retryAfterSeconds + "}"
            );
            return false; // stop the handler chain — controller is NOT called
        }

        // Attach remaining count as a response header (informational)
        if (result.remaining() >= 0) {
            response.setHeader("X-RateLimit-Remaining", String.valueOf(result.remaining()));
        }
        return true;
    }

    /**
     * Extract the rate limit key from the request.
     * Prefers the {@code X-API-Key} header; falls back to IP.
     */
    private static String extractKey(HttpServletRequest request) {
        String apiKey = request.getHeader("X-API-Key");
        if (apiKey != null && !apiKey.isBlank()) {
            return "apikey:" + apiKey;
        }
        // X-Forwarded-For is set by load balancers/proxies
        String forwarded = request.getHeader("X-Forwarded-For");
        if (forwarded != null && !forwarded.isBlank()) {
            return "ip:" + forwarded.split(",")[0].trim();
        }
        return "ip:" + request.getRemoteAddr();
    }

    private static long retryAfter(Instant resetAt) {
        if (resetAt == null) return 60L;
        long seconds = ChronoUnit.SECONDS.between(Instant.now(), resetAt);
        return Math.max(1L, seconds);
    }
}
