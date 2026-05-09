package dev.pushkar.flags;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.boot.CommandLineRunner;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.stereotype.Service;
import org.springframework.web.client.RestClientException;

/**
 * Demo application showing @FeatureFlag AOP in action.
 *
 * <p>To run this demo:
 * <pre>
 *   # Terminal 1: start the Go flag server
 *   cd labs/feature-flags
 *   go run ./cmd/server
 *
 *   # Terminal 2: run the Spring Boot demo
 *   cd labs/feature-flags/java-integration
 *   mvn spring-boot:run
 * </pre>
 *
 * <p>The demo:
 * <ol>
 *   <li>Calls {@code checkout()} — flag "new-checkout" is disabled by default → exception
 *   <li>Enables the flag via PUT /flags/new-checkout
 *   <li>Waits up to 2 seconds for the SSE push to update the cache
 *   <li>Calls {@code checkout()} again — now enabled → proceeds
 * </ol>
 */
@SpringBootApplication
public class FlagDemoApplication implements CommandLineRunner {

    private static final Logger log = LoggerFactory.getLogger(FlagDemoApplication.class);

    private final CheckoutService checkoutService;
    private final FlagClient flagClient;
    private final FlagCache flagCache;

    public FlagDemoApplication(CheckoutService checkoutService,
                                FlagClient flagClient,
                                FlagCache flagCache) {
        this.checkoutService = checkoutService;
        this.flagClient      = flagClient;
        this.flagCache       = flagCache;
    }

    public static void main(String[] args) {
        SpringApplication.run(FlagDemoApplication.class, args);
    }

    @Override
    public void run(String... args) throws Exception {
        log.info("=== Feature Flag Demo ===");
        log.info("Cached flags on startup: {}", flagCache.size());

        // Step 1: call checkout with flag disabled
        log.info("--- Step 1: checkout with 'new-checkout' DISABLED ---");
        try {
            String result = checkoutService.checkout("cart-001");
            log.info("checkout result: {}", result);
        } catch (FeatureDisabledException e) {
            log.info("Correctly blocked: {}", e.getMessage());
        }

        // Step 2: enable the flag via the Go server
        log.info("--- Step 2: enabling 'new-checkout' via PUT /flags/new-checkout ---");
        try {
            flagClient.updateFlag("new-checkout", true);
            log.info("Flag updated on server.");
        } catch (RestClientException e) {
            log.warn("Go server not available ({}). Enabling flag directly in cache.", e.getMessage());
            flagCache.put("new-checkout", true);
        }

        // Step 3: wait briefly for SSE push to propagate
        log.info("--- Step 3: waiting for SSE push (up to 2s) ---");
        Thread.sleep(2_000);

        // Step 4: call checkout again — should now proceed
        log.info("--- Step 4: checkout with 'new-checkout' ENABLED ---");
        try {
            String result = checkoutService.checkout("cart-001");
            log.info("checkout result: {}", result);
        } catch (FeatureDisabledException e) {
            log.warn("Flag still disabled (SSE push may not have arrived): {}", e.getMessage());
        }

        log.info("=== Demo complete ===");
    }
}

/**
 * Simulated checkout service — the @FeatureFlag annotation gates the method.
 * The aspect intercepts the call and checks the flag cache before proceeding.
 */
@Service
class CheckoutService {

    @FeatureFlag("new-checkout")
    public String checkout(String cartId) {
        return "New checkout completed for cart: " + cartId;
    }
}
