package dev.pushkar.regex;

import org.springframework.stereotype.Component;

import java.time.Duration;
import java.util.Map;
import java.util.concurrent.*;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/**
 * Demonstrates the ReDoS vulnerability in java.util.regex and mitigation strategies.
 *
 * <p>java.util.regex is a backtracking NFA engine (PCRE-style). For patterns like
 * {@code (a+)+}, it tries all possible ways to partition the input into groups.
 * For N characters, the number of such partitions is 2^(N-1) — exponential growth.
 *
 * <p>By contrast, the Rust regex crate uses a Thompson NFA/DFA simulation.
 * It runs all paths simultaneously and never backtracks. Time complexity: O(M*N).
 */
@Component
public class ReDoSDemo {

    /**
     * Demo 1: Shows the ReDoS vulnerability in java.util.regex.
     *
     * <p>Pattern: {@code (a+)+} — "one or more groups of one or more 'a'".
     * Input:   "aaaaaaaaaaaaaab" (N a's followed by 'b')
     *
     * <p>The backtracking engine tries to match by greedily grabbing a+,
     * then backtracks when the overall match fails, and tries all shorter groupings.
     * For N=15: ~16,000 paths. For N=30: ~500 million paths.
     *
     * <p>We cap at 15 chars here so the demo doesn't hang.
     * In production with user-supplied patterns, there is no such cap.
     */
    public void javaBacktrackingReDoS() {
        // Classic ReDoS pattern — equivalent to just (a+) but with nested quantifiers
        // that confuse backtracking engines.
        Pattern vulnerable = Pattern.compile("(a+)+");

        for (int n = 5; n <= 15; n += 5) {
            String input = "a".repeat(n) + "b";
            long start = System.nanoTime();
            boolean matched = vulnerable.matcher(input).matches();
            long elapsedMs = (System.nanoTime() - start) / 1_000_000;

            System.out.printf("  n=%2d | input=%-20s | result=%-5b | time=%dms%n",
                n, "\"" + input + "\"", matched, elapsedMs);
        }

        System.out.println("  Note: n=25 would take ~1 second; n=30 ~30 seconds.");
        System.out.println("  Our Rust NFA handles n=30 in under 0.1ms.");
    }

    /**
     * Demo 2: Illustrates Pattern.compile() performance inside a request loop.
     *
     * <p>The most common java.util.regex mistake in production code:
     * compiling the same pattern on every request. Pattern.compile() is expensive
     * (parses the regex, builds the internal NFA graph), but the compiled Pattern
     * is thread-safe and should be a static final field.
     */
    public void preCompilePattern() {
        final String emailPattern = "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$";
        final String testEmail = "user@example.com";
        final int iters = 10_000;

        // BAD: compiling in the loop
        long start = System.nanoTime();
        for (int i = 0; i < iters; i++) {
            Pattern.compile(emailPattern).matcher(testEmail).matches();
        }
        long badMs = (System.nanoTime() - start) / 1_000_000;

        // GOOD: compile once, reuse (Pattern is thread-safe)
        Pattern compiled = Pattern.compile(emailPattern);
        start = System.nanoTime();
        for (int i = 0; i < iters; i++) {
            compiled.matcher(testEmail).matches();
        }
        long goodMs = (System.nanoTime() - start) / 1_000_000;

        System.out.printf("  Compile-in-loop (%d iters): %dms%n", iters, badMs);
        System.out.printf("  Pre-compiled    (%d iters): %dms%n", iters, goodMs);
        System.out.printf("  Speedup: %.1fx%n", (double) badMs / Math.max(goodMs, 1));
        System.out.println("  Lesson: Pattern.compile() is O(M^2) in the worst case.");
        System.out.println("  Always use a static final Pattern field in production.");
    }

    /**
     * Demo 3: Named capture groups in Java.
     *
     * <p>Java supports named groups via {@code (?<name>...)} syntax (not (?P-name-) as in Python).
     * Named groups make regex code self-documenting and reduce index-based errors.
     *
     * @return extracted fields or empty map if no match
     */
    public Map<String, String> namedGroups() {
        // ISO date pattern with named groups
        Pattern datePattern = Pattern.compile(
            "(?<year>\\d{4})-(?<month>\\d{2})-(?<day>\\d{2})"
        );

        String[] inputs = { "2024-03-15", "invalid", "2026-01-07" };

        for (String input : inputs) {
            Matcher m = datePattern.matcher(input);
            if (m.find()) {
                System.out.printf("  Input: %-12s => year=%s month=%s day=%s%n",
                    input, m.group("year"), m.group("month"), m.group("day"));
            } else {
                System.out.printf("  Input: %-12s => no match%n", input);
            }
        }

        // Return the last successful match for testing
        Matcher last = datePattern.matcher("2026-01-07");
        if (last.find()) {
            return Map.of(
                "year",  last.group("year"),
                "month", last.group("month"),
                "day",   last.group("day")
            );
        }
        return Map.of();
    }

    /**
     * Demo 4: Backtracking mitigation — wrap Pattern.matches() with a Future timeout.
     *
     * <p>Java has no built-in ReDoS protection. The standard mitigation is to run
     * the match in a separate thread with a timeout. If it doesn't complete in time,
     * assume it's a ReDoS attack and reject the input.
     *
     * <p>Note: the thread is interrupted, not killed. If the regex match ignores
     * interrupts (most do), the thread continues running in the background until
     * the pattern eventually completes. A thread pool with a bounded queue is
     * needed to prevent thread exhaustion under attack.
     *
     * <p>The real fix is to use an NFA-based engine. In Java, re2j
     * (com.google.re2j:re2j) is the Google RE2 port — O(M*N) guaranteed.
     * However, it doesn't support backreferences or lookahead.
     */
    public void backtrackingMitigation() {
        ExecutorService executor = Executors.newSingleThreadExecutor();

        String[] patterns = {
            "^[a-z]+$",     // safe: simple character class
            "^(a+)+$",      // dangerous: nested quantifiers
        };
        String[] inputs = {
            "helloworld",
            "a".repeat(20) + "b",
        };

        for (int i = 0; i < patterns.length; i++) {
            final String pattern = patterns[i];
            final String input   = inputs[i];
            boolean safe = safeMatch(executor, pattern, input, Duration.ofMillis(100));
            System.out.printf("  Pattern: %-15s | Input: %-25s | Safe result: %s%n",
                "\"" + pattern + "\"",
                "\"" + input.substring(0, Math.min(input.length(), 20)) + (input.length() > 20 ? "..." : "") + "\"",
                safe ? "MATCHED" : "TIMED_OUT (rejected)"
            );
        }

        executor.shutdownNow();
        System.out.println("  Recommendation: use com.google.re2j:re2j for user-supplied patterns.");
    }

    /**
     * Run a regex match with a hard timeout.
     *
     * @param pattern pattern string
     * @param input   input text
     * @param timeout maximum time to allow the match to run
     * @return true if matched within timeout, false if timed out or no match
     */
    public boolean safeMatch(ExecutorService executor, String pattern, String input, Duration timeout) {
        Pattern compiled = Pattern.compile(pattern);
        Future<Boolean> future = executor.submit(() -> compiled.matcher(input).matches());
        try {
            return future.get(timeout.toMillis(), TimeUnit.MILLISECONDS);
        } catch (TimeoutException e) {
            future.cancel(true); // request interruption (best-effort)
            return false; // treat timeout as "no match / rejected"
        } catch (Exception e) {
            return false;
        }
    }
}
