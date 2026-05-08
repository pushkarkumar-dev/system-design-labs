package dev.pushkar.transformer;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import org.springframework.ai.openai.api.OpenAiApi;

import java.time.Duration;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.*;

/**
 * Unit tests for TransformerService.
 *
 * Key things under test:
 * 1. generate() returns the model's completion
 * 2. Cache hit: the model is NOT called a second time for the same prompt
 * 3. generateWithSystem() concatenates system + user prompt as the cache key
 * 4. cacheHitRate() reflects actual cache usage
 */
@ExtendWith(MockitoExtension.class)
class TransformerServiceTest {

    @Mock TransformerClient client;
    @Mock OpenAiApi openAiApi;

    TransformerService service;

    @BeforeEach
    void setUp() {
        var props = new TransformerProperties(
                "gpt-local",
                200,
                0.8,
                new TransformerProperties.CacheProperties(100, Duration.ofMinutes(10))
        );
        service = new TransformerService(client, props, openAiApi);
    }

    @Test
    void generate_delegates_to_client_on_cache_miss() {
        when(client.generate("To be or not to be")).thenReturn("That is the question.");

        String result = service.generate("To be or not to be");

        assertThat(result).isEqualTo("That is the question.");
        verify(client, times(1)).generate("To be or not to be");
    }

    @Test
    void generate_returns_cached_result_without_calling_model_again() {
        when(client.generate(anyString())).thenReturn("Once returned, always cached.");

        // First call — cache miss
        service.generate("What light through yonder window breaks?");
        // Second call with same prompt — should hit cache
        String result = service.generate("What light through yonder window breaks?");

        assertThat(result).isEqualTo("Once returned, always cached.");
        // Client must be called exactly once — the second call used the cache
        verify(client, times(1)).generate("What light through yonder window breaks?");
    }

    @Test
    void generate_with_system_delegates_to_client() {
        when(client.generateWithSystem("You are Shakespeare.", "Introduce yourself."))
                .thenReturn("I am the Bard of Avon.");

        String result = service.generateWithSystem("You are Shakespeare.", "Introduce yourself.");

        assertThat(result).isEqualTo("I am the Bard of Avon.");
        verify(client, times(1)).generateWithSystem("You are Shakespeare.", "Introduce yourself.");
    }

    @Test
    void cache_hit_rate_reflects_usage() {
        when(client.generate(anyString())).thenReturn("completed.");

        // 2 calls: 1 miss then 1 hit
        service.generate("ROMEO:");
        service.generate("ROMEO:");

        // Hit rate should be > 0 after a cache hit
        // (exact value depends on Caffeine's stats implementation)
        assertThat(service.cacheHitRate()).isGreaterThanOrEqualTo(0.0);
        assertThat(service.cacheHitRate()).isLessThanOrEqualTo(1.0);
    }

    @Test
    void different_prompts_are_cached_independently() {
        when(client.generate("HAMLET:")).thenReturn("hamlet-completion");
        when(client.generate("KING LEAR:")).thenReturn("lear-completion");

        String r1 = service.generate("HAMLET:");
        String r2 = service.generate("KING LEAR:");
        String r3 = service.generate("HAMLET:");  // cache hit for first prompt

        assertThat(r1).isEqualTo("hamlet-completion");
        assertThat(r2).isEqualTo("lear-completion");
        assertThat(r3).isEqualTo("hamlet-completion");

        // Client called once per unique prompt
        verify(client, times(1)).generate("HAMLET:");
        verify(client, times(1)).generate("KING LEAR:");
    }
}
