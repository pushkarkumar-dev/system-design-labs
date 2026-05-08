package dev.pushkar.transformer;

import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.context.annotation.Bean;

/**
 * Demo Spring Boot application that calls our locally-trained GPT model.
 *
 * <p>Prerequisites:
 * <ol>
 *   <li>Train the model: {@code cd labs/transformer/src && python train.py}
 *   <li>Start the server: {@code uvicorn server:app --host 0.0.0.0 --port 8000}
 *   <li>Run this: {@code mvn spring-boot:run}
 * </ol>
 *
 * <p>From this application's perspective, the model is just an OpenAI-compatible
 * LLM. Swapping it for GPT-4 requires changing only {@code application.yml}.
 */
@SpringBootApplication
public class TransformerDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(TransformerDemoApplication.class, args);
    }

    @Bean
    CommandLineRunner demo(TransformerService transformer) {
        return args -> {
            System.out.println("=== Transformer From Scratch — Spring AI Demo ===\n");

            // Shakespeare-style prompts — our model trained on TinyShakespeare
            // so these should produce the most coherent completions.
            String[] prompts = {
                "ROMEO:\nBut soft, what light through yonder",
                "HAMLET:\nTo be or not to be",
                "KING RICHARD:\nNow is the winter of",
            };

            for (String prompt : prompts) {
                System.out.println("Prompt: " + prompt.replace("\n", "\\n"));
                long t0 = System.currentTimeMillis();
                String completion = transformer.generate(prompt);
                long elapsed = System.currentTimeMillis() - t0;
                System.out.printf("Completion (%dms): %s%n%n",
                        elapsed, completion.replace("\n", "\\n"));
            }

            // The second call with the same prompt should hit the Caffeine cache
            System.out.println("--- Cached call (should be ~0ms) ---");
            long t0 = System.currentTimeMillis();
            transformer.generate(prompts[0]);
            System.out.printf("Cache hit took: %dms (vs ~50-200ms for a model call)%n%n",
                    System.currentTimeMillis() - t0);

            System.out.printf("Cache hit rate: %.1f%%%n", transformer.cacheHitRate() * 100);
            System.out.println("\nDone. Health: " + transformer.health().getStatus());
        };
    }
}
