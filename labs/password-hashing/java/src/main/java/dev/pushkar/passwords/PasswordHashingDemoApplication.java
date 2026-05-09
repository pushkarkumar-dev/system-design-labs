package dev.pushkar.passwords;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.boot.CommandLineRunner;
import org.springframework.context.annotation.Bean;
import org.springframework.security.crypto.argon2.Argon2PasswordEncoder;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.DelegatingPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;

import java.util.HashMap;
import java.util.Map;

/**
 * Spring Boot application demonstrating password hashing best practices.
 *
 * <p>Run with {@code mvn spring-boot:run} to see the interactive demo, or
 * {@code mvn test} to run the test suite.
 *
 * <p>The demo covers:
 * <ol>
 *   <li>bcrypt: hash with cost=10, verify, show self-describing format</li>
 *   <li>Argon2id: hash, verify, show format matches Go implementation</li>
 *   <li>PepperingPasswordEncoder: same password, different pepper = different hash</li>
 *   <li>PasswordUpgradeService: needsRehash flow with bcrypt → Argon2id migration</li>
 *   <li>DelegatingPasswordEncoder: reading {bcrypt} and {argon2} prefixed hashes</li>
 * </ol>
 */
@SpringBootApplication
public class PasswordHashingDemoApplication {

    public static void main(String[] args) {
        SpringApplication.run(PasswordHashingDemoApplication.class, args);
    }

    /**
     * CommandLineRunner that executes the demo on startup.
     *
     * <p>All output goes to stdout so you can see the results without a browser.
     */
    @Bean
    public CommandLineRunner demo() {
        return args -> {
            System.out.println("\n╔══════════════════════════════════════════════════════╗");
            System.out.println("║   Password Hashing Lab — Java Co-Primary Demo        ║");
            System.out.println("╚══════════════════════════════════════════════════════╝\n");

            demoBcrypt();
            demoArgon2id();
            demoPeppering();
            demoUpgradeService();
            demoDelegatingEncoder();
        };
    }

    private void demoBcrypt() {
        System.out.println("━━ 1. bcrypt (cost=10) ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
        BCryptPasswordEncoder encoder = new BCryptPasswordEncoder(10);

        long start = System.currentTimeMillis();
        String hash = encoder.encode("correct-horse-battery-staple");
        long elapsed = System.currentTimeMillis() - start;

        System.out.printf("  hash:    %s%n", hash);
        System.out.printf("  elapsed: %dms  (deliberate slowness — not a bug)%n", elapsed);
        System.out.printf("  verify:  %b  (correct password)%n", encoder.matches("correct-horse-battery-staple", hash));
        System.out.printf("  verify:  %b  (wrong password)%n", encoder.matches("wrong-password", hash));
        System.out.printf("  format:  $2a$<cost>$<22-char-salt><31-char-hash> — self-describing%n%n");
    }

    private void demoArgon2id() {
        System.out.println("━━ 2. Argon2id (m=64MB, t=3, p=2) ━━━━━━━━━━━━━━━━━━━━");
        // Use same params as Go DefaultArgon2Params() for interoperability
        Argon2PasswordEncoder encoder = new Argon2PasswordEncoder(16, 32, 2, 65536, 3);

        long start = System.currentTimeMillis();
        String hash = encoder.encode("my-secure-password");
        long elapsed = System.currentTimeMillis() - start;

        System.out.printf("  hash:    %s%n", hash);
        System.out.printf("  elapsed: %dms%n", elapsed);
        System.out.printf("  verify:  %b  (correct password)%n", encoder.matches("my-secure-password", hash));
        System.out.printf("  verify:  %b  (wrong password)%n", encoder.matches("wrong-password", hash));
        System.out.println("  format:  $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>");
        System.out.println("  note:    This format is identical to the Go implementation.");
        System.out.println("           A hash created by the Go server can be verified here.\n");
    }

    private void demoPeppering() {
        System.out.println("━━ 3. PepperingPasswordEncoder ━━━━━━━━━━━━━━━━━━━━━━━━");
        Argon2PasswordEncoder base = new Argon2PasswordEncoder(16, 32, 2, 65536, 3);

        PepperingPasswordEncoder encoderV1 = new PepperingPasswordEncoder(base, "pepper-v1-secret");
        PepperingPasswordEncoder encoderV2 = new PepperingPasswordEncoder(base, "pepper-v2-secret");

        String hash = encoderV1.encode("user-password");

        System.out.printf("  hash (pepper v1): %s%n", hash.substring(0, 60) + "...");
        System.out.printf("  verify v1:  %b  (correct password + correct pepper)%n",
                encoderV1.matches("user-password", hash));
        System.out.printf("  verify v2:  %b  (correct password + WRONG pepper)%n",
                encoderV2.matches("user-password", hash));
        System.out.println("  Stolen DB without pepper: attacker has hash, no pepper → uncrackable.\n");
    }

    private void demoUpgradeService() {
        System.out.println("━━ 4. PasswordUpgradeService — needsRehash flow ━━━━━━━━");

        // Old hash created with bcrypt cost=4 (test-only low cost)
        BCryptPasswordEncoder weakEncoder = new BCryptPasswordEncoder(4);
        String weakHash = weakEncoder.encode("user-password");
        System.out.printf("  old hash (bcrypt cost=4): %s%n", weakHash);

        // New encoder: Argon2id with pepper
        Argon2PasswordEncoder argon2 = new Argon2PasswordEncoder(16, 32, 2, 65536, 3);
        PepperingPasswordEncoder strongEncoder = new PepperingPasswordEncoder(argon2, "my-pepper");
        PasswordUpgradeService service = new PasswordUpgradeService(weakEncoder);

        // Verify with old (weak) encoder, then check if upgrade is needed
        PasswordUpgradeService.AuthResult result = service.authenticate("user-password", weakHash);
        System.out.printf("  authenticate: ok=%b, upgraded=%b%n", result.ok(), result.upgraded());

        // Now use PasswordUpgradeService with the strong encoder
        PasswordUpgradeService upgradeService = new PasswordUpgradeService(
                new BCryptPasswordEncoder(12) // upgraded cost
        );
        PasswordUpgradeService.AuthResult upgradeResult = upgradeService.authenticate("user-password", weakHash);
        System.out.printf("  with cost=12 encoder: ok=%b, upgraded=%b%n",
                upgradeResult.ok(), upgradeResult.upgraded());
        if (upgradeResult.upgraded()) {
            System.out.println("  → caller updates DB with new hash. User never noticed.");
        }
        System.out.println();
    }

    private void demoDelegatingEncoder() {
        System.out.println("━━ 5. DelegatingPasswordEncoder — multi-algorithm migration ━━");

        Map<String, PasswordEncoder> encoders = new HashMap<>();
        encoders.put("bcrypt", new BCryptPasswordEncoder(10));
        encoders.put("argon2", new Argon2PasswordEncoder(16, 32, 2, 65536, 3));

        // Default encoding: new hashes use argon2
        DelegatingPasswordEncoder delegating = new DelegatingPasswordEncoder("argon2", encoders);

        // New hash gets {argon2} prefix
        String newHash = delegating.encode("new-user-password");
        System.out.printf("  new hash format:  %s%n", newHash.substring(0, 50) + "...");
        System.out.printf("  starts with:      %s%n", newHash.substring(0, 8));

        // Old bcrypt hash (without prefix) — simulate legacy data
        BCryptPasswordEncoder bcrypt = new BCryptPasswordEncoder(10);
        String legacyHash = bcrypt.encode("legacy-user-password");
        // DelegatingPasswordEncoder needs the {bcrypt} prefix for old hashes
        String prefixedLegacy = "{bcrypt}" + legacyHash;

        System.out.printf("  legacy hash:      {bcrypt}%s...%n", legacyHash.substring(0, 20));
        System.out.printf("  verify legacy:    %b%n", delegating.matches("legacy-user-password", prefixedLegacy));
        System.out.printf("  verify new:       %b%n", delegating.matches("new-user-password", newHash));
        System.out.println("  Both algorithms verified by one encoder. Migration in place.");
    }
}
