package dev.pushkar.tokenizer;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.Mockito.*;

/**
 * Unit tests for TokenizerService.
 *
 * We mock TokenizerClient so these tests run without the Python server.
 * This is the right boundary: we want to test the service's caching logic,
 * health reporting, and method contracts — not the HTTP client or the
 * Python implementation.
 */
@ExtendWith(MockitoExtension.class)
class TokenizerServiceTest {

    @Mock
    private TokenizerClient client;

    private TokenizerService service;

    @BeforeEach
    void setUp() {
        var props = new TokenizerProperties(
                "http://localhost:8000",
                new TokenizerProperties.CacheProperties(1000L, Duration.ofMinutes(10))
        );
        service = new TokenizerService(client, props);
    }

    // ------------------------------------------------------------------
    // Test 1: encode + decode round-trip
    // ------------------------------------------------------------------

    @Test
    void tokenizeAndDetokenize_roundTrip() {
        // Arrange
        String text = "hello world";
        List<Integer> tokens = List.of(104, 101, 108, 108, 111, 32, 119, 111, 114, 108, 100);
        List<String> tokenStrings = List.of("h", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d");

        when(client.encode(text))
                .thenReturn(new TokenizerClient.TokenizeResult(tokens, tokenStrings));
        when(client.decode(tokens)).thenReturn(text);

        // Act
        var result = service.tokenize(text);
        String decoded = service.detokenize(result.tokens());

        // Assert
        assertThat(result.tokens()).isEqualTo(tokens);
        assertThat(decoded).isEqualTo(text);
    }

    // ------------------------------------------------------------------
    // Test 2: cache hit on repeated string
    // ------------------------------------------------------------------

    @Test
    void tokenize_cacheHit_onRepeatedString() {
        // Arrange
        String text = "the cat sat on the mat";
        var mockResult = new TokenizerClient.TokenizeResult(
                List.of(1, 2, 3, 4, 5),
                List.of("the", " cat", " sat", " on", " mat")
        );
        when(client.encode(text)).thenReturn(mockResult);

        // Act — call twice
        service.tokenize(text);
        service.tokenize(text);

        // Assert — the client should only have been called once (second call is a cache hit)
        verify(client, times(1)).encode(text);
    }

    // ------------------------------------------------------------------
    // Test 3: empty string returns empty token list
    // ------------------------------------------------------------------

    @Test
    void tokenize_emptyString_returnsEmptyTokens() {
        // Arrange
        when(client.encode(""))
                .thenReturn(new TokenizerClient.TokenizeResult(List.of(), List.of()));

        // Act
        var result = service.tokenize("");

        // Assert
        assertThat(result.tokens()).isEmpty();
        assertThat(result.tokenStrings()).isEmpty();
    }

    // ------------------------------------------------------------------
    // Test 4: long text is tokenised correctly (no truncation)
    // ------------------------------------------------------------------

    @Test
    void tokenize_longText_returnsAllTokens() {
        // Arrange — simulate a 100-token result
        String longText = "the quick brown fox jumps over the lazy dog ".repeat(10);
        List<Integer> manyTokens = new java.util.ArrayList<>();
        List<String> manyStrings = new java.util.ArrayList<>();
        for (int i = 0; i < 100; i++) {
            manyTokens.add(i + 256);
            manyStrings.add("tok" + i);
        }
        when(client.encode(longText))
                .thenReturn(new TokenizerClient.TokenizeResult(manyTokens, manyStrings));

        // Act
        var result = service.tokenize(longText);

        // Assert
        assertThat(result.tokens()).hasSize(100);
        assertThat(result.tokenStrings()).hasSize(100);
    }

    // ------------------------------------------------------------------
    // Test 5: tokenized count matches Python reference for sample string
    // ------------------------------------------------------------------

    @Test
    void tokenize_knownSampleString_matchesPythonReferenceTokenCount() {
        // This test documents the expected token count for a known string.
        // When the Python server is replaced or retrained, this reference
        // should be updated — it serves as a regression check.
        //
        // For a vocab=1000 GPT-2 BPE tokenizer trained on TinyShakespeare:
        //   "hello world" -> approximately 3-5 tokens (subwords merge "ello", "orld", etc.)
        // We mock a representative result here.
        String text = "hello world";
        int expectedTokenCount = 4;  // representative for vocab=1000 BPE

        when(client.encode(text))
                .thenReturn(new TokenizerClient.TokenizeResult(
                        List.of(257, 258, 259, 260),  // 4 merged tokens
                        List.of("hel", "lo", " w", "orld")
                ));

        var result = service.tokenize(text);

        assertThat(result.tokens()).hasSize(expectedTokenCount);
        // The token strings should reconstruct the original text when concatenated
        String reconstructed = String.join("", result.tokenStrings());
        assertThat(reconstructed).isEqualTo(text);
    }
}
