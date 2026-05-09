package dev.pushkar.tokenizer;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.context.event.ApplicationReadyEvent;
import org.springframework.context.event.EventListener;

import java.util.List;

/**
 * Spring Boot demo application for the BPE tokenizer integration.
 *
 * On startup (after the server is ready), runs a brief demo that:
 *   1. Checks the tokenizer server's health.
 *   2. Tokenizes several strings and shows the tokens.
 *   3. Decodes the tokens back to verify round-trip correctness.
 *   4. Tokenizes the same string twice to demonstrate cache hits.
 *
 * Prerequisites:
 *   1. Train a tokenizer and start the Python server:
 *        cd labs/tokenizer
 *        python src/train.py          # creates /tmp/gpt2bpe_tinysearch.json
 *        uvicorn src.server:app --port 8000
 *   2. Run this application:
 *        cd labs/tokenizer/java-integration
 *        mvn spring-boot:run
 */
@SpringBootApplication
public class TokenizerDemoApplication {

    private final TokenizerService tokenizerService;

    public TokenizerDemoApplication(TokenizerService tokenizerService) {
        this.tokenizerService = tokenizerService;
    }

    public static void main(String[] args) {
        SpringApplication.run(TokenizerDemoApplication.class, args);
    }

    @EventListener(ApplicationReadyEvent.class)
    public void onReady() {
        System.out.println("\n=== BPE Tokenizer Spring Integration Demo ===\n");

        // 1. Health check
        try {
            var health = tokenizerService.health();
            System.out.println("Tokenizer health: " + health.getStatus());
            System.out.println("  Vocab size: " + tokenizerService.getVocabSize());
        } catch (Exception e) {
            System.out.println("WARNING: Tokenizer server unreachable — " + e.getMessage());
            System.out.println("  Start the Python server: uvicorn src.server:app --port 8000");
            return;
        }

        // 2. Tokenize a few strings
        List<String> testStrings = List.of(
                "hello world",
                "don't log the catalog",
                "tokenization is compression",
                "the cat sat on the mat"
        );

        System.out.println("\nTokenizing test strings:");
        for (String text : testStrings) {
            var result = tokenizerService.tokenize(text);
            System.out.printf("  %-35s -> %d tokens%n", "\"" + text + "\"", result.tokens().size());
        }

        // 3. Decode round-trip
        System.out.println("\nRound-trip decode check:");
        for (String text : testStrings) {
            var result = tokenizerService.tokenize(text);
            String decoded = tokenizerService.detokenize(result.tokens());
            String status = decoded.equals(text) ? "OK" : "MISMATCH";
            System.out.printf("  [%s] %s%n", status, text);
        }

        // 4. Cache hit demo — tokenize the same string twice
        String repeated = "hello world";
        tokenizerService.tokenize(repeated);  // first call — cache miss
        tokenizerService.tokenize(repeated);  // second call — cache hit
        System.out.printf("%nCache hit rate after repeated call: %.1f%%%n",
                tokenizerService.cacheHitRatePct());

        System.out.println("\nDone. Actuator health: http://localhost:8080/actuator/health");
    }
}
