package dev.pushkar.tokenizer;

import com.github.benmanes.caffeine.cache.Cache;
import com.github.benmanes.caffeine.cache.Caffeine;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Service;

import java.util.List;

/**
 * Application-level tokenizer service with Caffeine in-process caching.
 *
 * This service wraps TokenizerClient and adds:
 *   1. Caching — repeated tokenization of the same string hits the cache,
 *      not the Python server. This matters for prompt templates and system
 *      messages that are sent with every request.
 *   2. Health reporting — implements HealthIndicator so Spring Actuator
 *      includes tokenizer status in /actuator/health alongside JVM health.
 *
 * Caching strategy: write-through on tokenize(), invalidate on vocab reload.
 * The cache key is the raw input string. This is safe because BPE tokenization
 * is deterministic — the same string always produces the same token IDs.
 *
 * Why Caffeine?
 *   Caffeine's W-TinyLFU eviction policy has much better hit rates than LRU
 *   for tokenization workloads, where a small set of prompt templates are
 *   tokenized thousands of times while new user messages are tokenized once.
 */
@Service
public class TokenizerService implements HealthIndicator {

    private final TokenizerClient client;
    private final Cache<String, TokenizerClient.TokenizeResult> cache;

    public TokenizerService(TokenizerClient client, TokenizerProperties props) {
        this.client = client;
        this.cache = Caffeine.newBuilder()
                .maximumSize(props.cache().maxEntries())
                .expireAfterWrite(props.cache().ttl())
                .recordStats()           // enables hit-rate reporting via Actuator
                .build();
    }

    /**
     * Tokenize text.
     *
     * Returns cached result if available; otherwise calls the Python server
     * and caches the result.
     *
     * @param text  Input string to tokenize.
     * @return      TokenizeResult containing IDs and their string forms.
     */
    public TokenizerClient.TokenizeResult tokenize(String text) {
        return cache.get(text, client::encode);
    }

    /**
     * Reconstruct the original string from token IDs.
     *
     * Decoding is not cached — it's typically only done once per token sequence
     * and the sequence is the natural key, not the string.
     *
     * @param tokens  List of integer token IDs.
     * @return        The decoded string.
     */
    public String detokenize(List<Integer> tokens) {
        return client.decode(tokens);
    }

    /**
     * Returns the vocabulary size of the loaded tokenizer.
     *
     * Useful for model configuration: the embedding table must have at least
     * vocab_size rows.
     */
    public int getVocabSize() {
        return client.vocabSize();
    }

    /**
     * Invalidate the entire cache.
     *
     * Call this after reloading the tokenizer vocabulary — cached results
     * from the old vocab would produce wrong IDs with the new one.
     */
    public void invalidateCache() {
        cache.invalidateAll();
    }

    /**
     * Returns the current cache hit rate as a percentage (0.0 to 100.0).
     *
     * A high hit rate means the in-process cache is effective and we're
     * saving significant HTTP round-trips. A low hit rate suggests every
     * request has unique text — consider disabling the cache to save memory.
     */
    public double cacheHitRatePct() {
        return cache.stats().hitRate() * 100.0;
    }

    // ------------------------------------------------------------------
    // Actuator health indicator
    // ------------------------------------------------------------------

    /**
     * Surfaces tokenizer status in /actuator/health.
     *
     * A "down" status here means the Python server is unreachable — the
     * Spring app can still start, but tokenization calls will fail.
     */
    @Override
    public Health health() {
        try {
            var h = client.health();
            return Health.up()
                    .withDetail("vocabSize", h.vocab_size())
                    .withDetail("cacheHitRate",
                            String.format("%.1f%%", cacheHitRatePct()))
                    .withDetail("cacheSize", cache.estimatedSize())
                    .build();
        } catch (Exception e) {
            return Health.down()
                    .withException(e)
                    .withDetail("message", "Cannot reach Python tokenizer server")
                    .build();
        }
    }
}
