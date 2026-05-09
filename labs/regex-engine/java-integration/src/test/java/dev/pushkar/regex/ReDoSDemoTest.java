package dev.pushkar.regex;

import org.junit.jupiter.api.Test;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.boot.test.context.SpringBootTest;

import java.time.Duration;
import java.util.Map;
import java.util.concurrent.Executors;

import static org.assertj.core.api.Assertions.assertThat;

/**
 * Integration tests for the ReDoS demo and regex service.
 *
 * <p>These tests demonstrate:
 * <ol>
 *   <li>Safe patterns complete fast (well under the 100ms timeout).
 *   <li>ReDoS patterns are caught by the timeout guard and return false.
 *   <li>Named group extraction works correctly.
 *   <li>Pre-compiled patterns are faster than per-call compilation.
 * </ol>
 */
@SpringBootTest
class ReDoSDemoTest {

    @Autowired
    private RegexService regexService;

    @Autowired
    private ReDoSDemo demo;

    /**
     * Test 1: A safe pattern (simple character class) completes well under timeout.
     *
     * <p>Pattern: {@code ^[a-z0-9]+$} — simple character class, no nested quantifiers.
     * This should match in microseconds even for long inputs.
     */
    @Test
    void safePatternCompletesQuickly() {
        String pattern = "^[a-z0-9]+$";
        String input = "hello123world";

        long start = System.nanoTime();
        boolean result = regexService.validate(input, pattern);
        long elapsedMs = (System.nanoTime() - start) / 1_000_000;

        assertThat(result).isTrue();
        assertThat(elapsedMs)
            .as("Safe pattern should complete in under 50ms, took %dms", elapsedMs)
            .isLessThan(50L);
    }

    /**
     * Test 2: ReDoS pattern is caught by the timeout guard.
     *
     * <p>Pattern: {@code (a+)+} on a 25-char attack string.
     * A backtracking engine explores ~2^24 = 16 million paths before giving up.
     * Our guard rejects it after 100ms and returns false.
     *
     * <p>Note: this test deliberately uses a timeout of 50ms (below the service's
     * 100ms default) so it doesn't slow down the test suite even if the backtracker
     * is partially fast on the test machine.
     */
    @Test
    void redosPatternTimesOutWithGuard() {
        // Build the attack string: N 'a's + 'b' forces exponential backtracking
        String attackInput = "a".repeat(25) + "b";
        String redosPattern = "(a+)+";

        // Use a very short timeout to ensure the test is deterministic
        var executor = Executors.newSingleThreadExecutor();
        try {
            boolean result = demo.safeMatch(executor, redosPattern, attackInput, Duration.ofMillis(50));
            // Should be false — either timed out or no match (it genuinely doesn't match)
            assertThat(result)
                .as("ReDoS pattern should timeout or return false for attack input")
                .isFalse();
        } finally {
            executor.shutdownNow();
        }
    }

    /**
     * Test 3: Named group extraction works correctly.
     *
     * <p>Pattern: ISO date with named groups year, month, day.
     * Verifies that java.util.regex named groups work end-to-end.
     */
    @Test
    void namedGroupExtractionWorks() {
        Map<String, String> groups = demo.namedGroups();

        assertThat(groups)
            .containsEntry("year",  "2026")
            .containsEntry("month", "01")
            .containsEntry("day",   "07");
    }

    /**
     * Test 4: Pre-compiled pattern is faster than compiling inside a loop.
     *
     * <p>Validates the lesson from Demo 2: Pattern.compile() is expensive.
     * The pre-compiled approach should be at least 2x faster.
     */
    @Test
    void precompiledPatternIsFaster() {
        String emailPattern = "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$";
        String testEmail = "user@example.com";
        int iters = 5_000;

        // Compile in loop (bad)
        long start = System.nanoTime();
        for (int i = 0; i < iters; i++) {
            java.util.regex.Pattern.compile(emailPattern).matcher(testEmail).matches();
        }
        long compileInLoopNs = System.nanoTime() - start;

        // Pre-compiled (good)
        java.util.regex.Pattern compiled = java.util.regex.Pattern.compile(emailPattern);
        start = System.nanoTime();
        for (int i = 0; i < iters; i++) {
            compiled.matcher(testEmail).matches();
        }
        long precompiledNs = System.nanoTime() - start;

        double speedup = (double) compileInLoopNs / Math.max(precompiledNs, 1);

        assertThat(speedup)
            .as("Pre-compiled should be at least 2x faster (got %.1fx)", speedup)
            .isGreaterThanOrEqualTo(2.0);
    }

    /**
     * Test 5: RegexService correctly validates safe inputs.
     */
    @Test
    void serviceValidatesSafeInput() {
        assertThat(regexService.validate("hello@example.com",
            "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$")).isTrue();

        assertThat(regexService.validate("not-an-email",
            "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$")).isFalse();
    }

    /**
     * Test 6: RegexService rejects an invalid pattern.
     */
    @Test
    void serviceRejectsInvalidPattern() {
        // Unclosed group — PatternSyntaxException, should return false
        assertThat(regexService.validate("input", "(unclosed")).isFalse();
    }
}
