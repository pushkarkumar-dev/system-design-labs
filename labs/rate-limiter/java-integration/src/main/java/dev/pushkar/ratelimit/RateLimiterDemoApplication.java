package dev.pushkar.ratelimit;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestHeader;
import org.springframework.web.bind.annotation.RestController;

import java.util.Map;

/**
 * Demo Spring Boot application with two endpoints:
 *
 * <ul>
 *   <li>{@code GET /api/data} — rate-limited (via the interceptor); limit is 10/min in demo config
 *   <li>{@code GET /public/ping} — not rate-limited (excluded in interceptor path config)
 *   <li>{@code GET /actuator/health} — Spring Actuator health endpoint (always excluded)
 * </ul>
 *
 * <h3>How to observe rate limiting</h3>
 * <ol>
 *   <li>Start the Go server: {@code LIMITER=distributed REDIS_ADDR=localhost:6379 go run ./cmd/server}
 *   <li>Start this app: {@code mvn spring-boot:run}
 *   <li>Send 11 requests: {@code for i in $(seq 1 11); do curl -s -o /dev/null -w "%{http_code}\n" -H "X-API-Key: demo" http://localhost:8081/api/data; done}
 *   <li>The first 10 return 200; the 11th returns 429 with a Retry-After header.
 * </ol>
 */
@SpringBootApplication
public class RateLimiterDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(RateLimiterDemoApplication.class, args);
    }

    @RestController
    static class DemoController {

        /**
         * Rate-limited endpoint. The interceptor checks the X-API-Key header
         * (or falls back to IP) before this method is ever called.
         *
         * If the limit is exceeded, Spring returns 429 before reaching here.
         */
        @GetMapping("/api/data")
        public ResponseEntity<Map<String, String>> getData(
                @RequestHeader(value = "X-API-Key", required = false) String apiKey) {
            return ResponseEntity.ok(Map.of(
                "message", "Hello, " + (apiKey != null ? apiKey : "anonymous") + "!",
                "note", "This endpoint is rate-limited to 10 req/min in demo config"
            ));
        }

        /**
         * Public endpoint — not rate-limited.
         * The interceptor's path exclusion list must include /public/** for this to work.
         */
        @GetMapping("/public/ping")
        public ResponseEntity<Map<String, String>> ping() {
            return ResponseEntity.ok(Map.of("status", "ok", "note", "This endpoint is NOT rate-limited"));
        }
    }
}
