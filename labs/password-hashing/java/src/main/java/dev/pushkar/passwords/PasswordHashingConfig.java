package dev.pushkar.passwords;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.crypto.argon2.Argon2PasswordEncoder;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.DelegatingPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;

import java.util.HashMap;
import java.util.Map;

/**
 * Spring configuration for the password hashing service.
 *
 * <p>The key component is {@link DelegatingPasswordEncoder}, which supports multiple
 * encoding formats simultaneously. Each stored hash has a format prefix:
 * {@code {bcrypt}$2a$10$...} or {@code {argon2}$argon2id$v=19$...}.
 *
 * <p>This makes zero-downtime algorithm migration possible:
 * <ol>
 *   <li>Old users have {@code {bcrypt}...} hashes. They still log in normally.</li>
 *   <li>New hashes are created with {@code {argon2}...} (the default encoder).</li>
 *   <li>On login, bcrypt users' hashes are upgraded to Argon2id transparently.</li>
 *   <li>Eventually, all hashes are Argon2id. Remove bcrypt from the delegate map.</li>
 * </ol>
 *
 * <p>Without {@link DelegatingPasswordEncoder}, algorithm migration requires a flag day:
 * all users must reset their passwords at the same time. That's disruptive and insecure
 * (users reuse passwords, the old hashes live in DB backups). DelegatingPasswordEncoder
 * is the correct production approach.
 */
@Configuration
public class PasswordHashingConfig {

    /**
     * The pepper secret, loaded from the environment.
     * In production: use AWS Secrets Manager, HashiCorp Vault, or a KMS.
     * Never store the pepper in the database or version control.
     */
    @Value("${app.security.pepper:default-insecure-pepper-change-in-prod}")
    private String pepper;

    /**
     * The primary password encoder: Argon2id with OWASP-recommended parameters,
     * wrapped in a {@link PepperingPasswordEncoder}.
     *
     * <p>This is what {@link PasswordUpgradeService} uses. New hashes always use this.
     *
     * @return a {@link PepperingPasswordEncoder} wrapping Argon2id
     */
    @Bean
    public PasswordEncoder passwordEncoder() {
        // Argon2id: m=65536 (64MB), t=3, p=4 — OWASP 2023 recommendation.
        // These match the Go DefaultArgon2Params() exactly.
        Argon2PasswordEncoder argon2 = new Argon2PasswordEncoder(
                16,     // saltLength bytes
                32,     // hashLength bytes
                2,      // parallelism (Spring Security default; OWASP recommends 4)
                65536,  // memory KiB — 64MB
                3       // iterations
        );
        return new PepperingPasswordEncoder(argon2, pepper);
    }

    /**
     * A {@link DelegatingPasswordEncoder} that supports multiple algorithms for migration.
     *
     * <p>Hash format: {@code {id}encodedHash}. Examples:
     * <ul>
     *   <li>{@code {bcrypt}$2a$10$...} — legacy bcrypt hash</li>
     *   <li>{@code {argon2}$argon2id$v=19$...} — current Argon2id hash</li>
     * </ul>
     *
     * <p>The default encoding ID is "argon2" — new hashes use Argon2id.
     * Old bcrypt hashes are still verified by the delegating encoder.
     *
     * <p>Usage: inject this where you need to read both formats from the DB,
     * typically at the AuthController/UserService level during migration.
     *
     * @return a {@link DelegatingPasswordEncoder} with bcrypt and argon2 delegates
     */
    @Bean("delegatingPasswordEncoder")
    public PasswordEncoder delegatingPasswordEncoder() {
        Map<String, PasswordEncoder> encoders = new HashMap<>();

        // Legacy bcrypt (cost=10) — reads {bcrypt}$2a$10$... hashes from DB
        encoders.put("bcrypt", new BCryptPasswordEncoder(10));

        // Current Argon2id with pepper — reads {argon2}$argon2id$... hashes from DB
        Argon2PasswordEncoder argon2 = new Argon2PasswordEncoder(16, 32, 2, 65536, 3);
        encoders.put("argon2", new PepperingPasswordEncoder(argon2, pepper));

        // Default encoding: new hashes get the {argon2} prefix
        DelegatingPasswordEncoder delegating = new DelegatingPasswordEncoder("argon2", encoders);

        // Passwords without a prefix (legacy, pre-DelegatingPasswordEncoder)
        // are treated as bcrypt. This covers the migration case where you had
        // raw bcrypt hashes without the {bcrypt} prefix.
        delegating.setDefaultPasswordEncoderForMatches(new BCryptPasswordEncoder(10));

        return delegating;
    }
}
