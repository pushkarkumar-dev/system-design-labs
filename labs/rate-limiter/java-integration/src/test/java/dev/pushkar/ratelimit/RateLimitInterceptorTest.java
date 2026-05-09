package dev.pushkar.ratelimit;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;
import org.springframework.web.client.ResourceAccessException;

import java.time.Instant;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.*;

/**
 * Unit tests for RateLimitInterceptor.
 *
 * Tests:
 * 1. Under-limit request passes (preHandle returns true)
 * 2. Over-limit request gets 429 (preHandle returns false)
 * 3. Retry-After header set correctly on 429
 * 4. X-API-Key header extraction
 * 5. Disabled interceptor always passes
 */
@ExtendWith(MockitoExtension.class)
class RateLimitInterceptorTest {

    @Mock
    private RateLimiterClient client;

    private RateLimiterProperties enabledProps;
    private RateLimiterProperties disabledProps;

    @BeforeEach
    void setUp() {
        enabledProps  = new RateLimiterProperties("http://localhost:8080", "free", true);
        disabledProps = new RateLimiterProperties("http://localhost:8080", "free", false);
    }

    // ── Test 1: Under-limit request passes ───────────────────────────────────

    @Test
    void underLimit_requestPassesThrough() throws Exception {
        var result = new RateLimiterClient.RateLimitResult(true, 42L, null);
        when(client.check(any(), any())).thenReturn(result);

        var interceptor = new RateLimitInterceptor(client, enabledProps);
        var request  = new MockHttpServletRequest("GET", "/api/data");
        request.addHeader("X-API-Key", "user-abc");
        var response = new MockHttpServletResponse();

        boolean proceed = interceptor.preHandle(request, response, new Object());

        assertThat(proceed).isTrue();
        assertThat(response.getStatus()).isEqualTo(200); // default — not overwritten
        assertThat(response.getHeader("X-RateLimit-Remaining")).isEqualTo("42");
    }

    // ── Test 2: Over-limit request gets 429 ─────────────────────────────────

    @Test
    void overLimit_returns429() throws Exception {
        var resetAt = Instant.now().plusSeconds(30);
        var result  = new RateLimiterClient.RateLimitResult(false, 0L, resetAt);
        when(client.check(any(), any())).thenReturn(result);

        var interceptor = new RateLimitInterceptor(client, enabledProps);
        var request  = new MockHttpServletRequest("GET", "/api/data");
        request.addHeader("X-API-Key", "spammer");
        var response = new MockHttpServletResponse();

        boolean proceed = interceptor.preHandle(request, response, new Object());

        assertThat(proceed).isFalse();
        assertThat(response.getStatus()).isEqualTo(429);
    }

    // ── Test 3: Retry-After header set correctly ─────────────────────────────

    @Test
    void overLimit_retryAfterHeaderSet() throws Exception {
        // Reset is 45 seconds from now
        var resetAt = Instant.now().plusSeconds(45);
        var result  = new RateLimiterClient.RateLimitResult(false, 0L, resetAt);
        when(client.check(any(), any())).thenReturn(result);

        var interceptor = new RateLimitInterceptor(client, enabledProps);
        var request  = new MockHttpServletRequest("GET", "/api/data");
        var response = new MockHttpServletResponse();

        interceptor.preHandle(request, response, new Object());

        String retryAfter = response.getHeader("Retry-After");
        assertThat(retryAfter).isNotNull();
        long seconds = Long.parseLong(retryAfter);
        // Should be between 40 and 50 seconds (tolerating a few seconds of test execution time)
        assertThat(seconds).isBetween(40L, 50L);
    }

    // ── Test 4: X-API-Key header extraction ─────────────────────────────────

    @Test
    void xApiKey_usedAsRateLimitKey() throws Exception {
        var result = new RateLimiterClient.RateLimitResult(true, 99L, null);
        when(client.check(eq("apikey:my-key-123"), any())).thenReturn(result);

        var interceptor = new RateLimitInterceptor(client, enabledProps);
        var request  = new MockHttpServletRequest("GET", "/api/data");
        request.addHeader("X-API-Key", "my-key-123");
        var response = new MockHttpServletResponse();

        boolean proceed = interceptor.preHandle(request, response, new Object());

        assertThat(proceed).isTrue();
        // Verify the client was called with the key prefixed as "apikey:"
        verify(client).check(eq("apikey:my-key-123"), any());
    }

    // ── Test 5: Disabled interceptor always passes ───────────────────────────

    @Test
    void disabled_alwaysPasses_withoutCallingClient() throws Exception {
        var interceptor = new RateLimitInterceptor(client, disabledProps);
        var request  = new MockHttpServletRequest("GET", "/api/data");
        var response = new MockHttpServletResponse();

        boolean proceed = interceptor.preHandle(request, response, new Object());

        assertThat(proceed).isTrue();
        // The client should never be called when disabled
        verify(client, never()).check(any(), any());
    }

    // ── Bonus: Fail-open when service is unreachable ─────────────────────────

    @Test
    void serviceUnreachable_failsOpen() throws Exception {
        when(client.check(any(), any()))
            .thenThrow(new ResourceAccessException("Connection refused"));

        var interceptor = new RateLimitInterceptor(client, enabledProps);
        var request  = new MockHttpServletRequest("GET", "/api/data");
        var response = new MockHttpServletResponse();

        boolean proceed = interceptor.preHandle(request, response, new Object());

        // Fail open: allow the request through, don't throw
        assertThat(proceed).isTrue();
    }
}
