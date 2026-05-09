package dev.pushkar.regex;

import org.springframework.stereotype.Service;

import java.time.Duration;
import java.util.Map;
import java.util.concurrent.*;
import java.util.regex.Pattern;
import java.util.regex.PatternSyntaxException;

/**
 * Production-safe regex validation service.
 *
 * <p>java.util.regex uses a backtracking NFA under the hood. This means:
 * <ol>
 *   <li>Nested quantifiers like {@code (a+)+} can exhibit catastrophic backtracking
 *       — O(2^N) time for N-character input.
 *   <li>User-supplied patterns are especially dangerous: an attacker can craft a
 *       pattern that hangs the JVM thread for minutes.
 *   <li>The mitigation is to run matches in a dedicated executor with a timeout.
 * </ol>
 *
 * <p>For a permanent fix, consider {@code com.google.re2j:re2j} which is based on
 * RE2 (Google's Thompson NFA implementation). It guarantees O(M*N) time for all
 * patterns, at the cost of not supporting backreferences and lookahead.
 *
 * <p>Our Rust engine in {@code labs/regex-engine/} is also O(M*N) guaranteed.
 */
@Service
public class RegexService {

    /**
     * Executor for running potentially-dangerous regex matches.
     *
     * <p>Single-threaded to enforce one match at a time. In a real system,
     * use a bounded thread pool (e.g., 4 threads) to allow parallelism while
     * capping resource usage under attack.
     */
    private final ExecutorService matchExecutor = Executors.newFixedThreadPool(2);

    private final long matchTimeoutMs;

    public RegexService(RegexProperties props) {
        this.matchTimeoutMs = props.matchTimeoutMs();
    }

    /**
     * Validate {@code input} against {@code pattern} with a timeout guard.
     *
     * <p>Returns false (not matched) in three cases:
     * <ol>
     *   <li>The pattern is syntactically invalid.
     *   <li>The input does not match the pattern.
     *   <li>The match takes longer than {@code matchTimeoutMs} ms (potential ReDoS).
     * </ol>
     *
     * <p>Never throws — safe to call from a request handler without try/catch.
     *
     * @param input   the string to test
     * @param pattern the regex pattern (may be user-supplied)
     * @return true if matched within timeout, false otherwise
     */
    public boolean validate(String input, String pattern) {
        if (input == null || pattern == null) return false;

        final Pattern compiled;
        try {
            // UNICODE_CHARACTER_CLASS: use Unicode-aware character classes.
            // This makes \w match Unicode word characters, not just ASCII [a-zA-Z0-9_].
            // Slightly slower but correct for international input.
            compiled = Pattern.compile(pattern, Pattern.UNICODE_CHARACTER_CLASS);
        } catch (PatternSyntaxException e) {
            // Invalid pattern — reject immediately
            return false;
        }

        Future<Boolean> future = matchExecutor.submit(
            () -> compiled.matcher(input).matches()
        );

        try {
            return Boolean.TRUE.equals(future.get(matchTimeoutMs, TimeUnit.MILLISECONDS));
        } catch (TimeoutException e) {
            // Match took too long — likely a ReDoS attack or pathological pattern.
            // Cancel the task (best-effort — the thread may still be running).
            future.cancel(true);
            return false;
        } catch (Exception e) {
            return false;
        }
    }

    /**
     * Extract named capture groups from {@code input} using {@code pattern}.
     *
     * <p>Returns an empty map if the input doesn't match or the match times out.
     *
     * @param input        the string to match
     * @param pattern      a regex with named groups: {@code (?<name>...)}
     * @param groupNames   the group names to extract
     */
    public Map<String, String> extractGroups(String input, String pattern, String... groupNames) {
        if (input == null || pattern == null) return Map.of();

        final Pattern compiled;
        try {
            compiled = Pattern.compile(pattern);
        } catch (PatternSyntaxException e) {
            return Map.of();
        }

        Future<Map<String, String>> future = matchExecutor.submit(() -> {
            var matcher = compiled.matcher(input);
            if (!matcher.find()) return Map.of();
            var result = new java.util.HashMap<String, String>();
            for (String name : groupNames) {
                try {
                    String value = matcher.group(name);
                    if (value != null) result.put(name, value);
                } catch (IllegalArgumentException ignored) {
                    // Group name not in pattern
                }
            }
            return Map.copyOf(result);
        });

        try {
            var result = future.get(matchTimeoutMs, TimeUnit.MILLISECONDS);
            return result != null ? result : Map.of();
        } catch (TimeoutException e) {
            future.cancel(true);
            return Map.of();
        } catch (Exception e) {
            return Map.of();
        }
    }
}
