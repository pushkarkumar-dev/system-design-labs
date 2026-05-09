package dev.pushkar.passwords;

import org.springframework.security.crypto.password.PasswordEncoder;
import org.springframework.stereotype.Service;

/**
 * Service that authenticates users and transparently upgrades password hashes.
 *
 * <p>Transparent hash upgrades let you improve password security without
 * disrupting users. On every successful login:
 * <ol>
 *   <li>Verify the stored hash with the current encoder.</li>
 *   <li>If {@code upgradeEncoding()} returns {@code true}, the stored hash was
 *       created with outdated parameters (lower bcrypt cost, weaker Argon2id params,
 *       or an old pepper version).</li>
 *   <li>Re-hash the raw password with the current encoder and update the DB.</li>
 * </ol>
 *
 * <p>The user never notices. Their password didn't change. The stored hash quietly
 * gets stronger over time as they log in. Users who haven't logged in since the
 * parameter upgrade still have the old (weaker) hash — but that's acceptable because
 * the hash is still correct, just not at the current security level.
 *
 * <p>This is the same logic as the Go {@code PepperKeyStore.Verify()} returning
 * {@code NeedsRehash: true}. Both implementations signal the same thing: the caller
 * should re-hash on successful login.
 */
@Service
public class PasswordUpgradeService {

    private final PasswordEncoder encoder;

    /**
     * Creates the service with the given encoder.
     *
     * <p>In production, inject a {@link PepperingPasswordEncoder} wrapping
     * {@link org.springframework.security.crypto.argon2.Argon2PasswordEncoder}.
     * The encoder is the source of truth for "current" parameters.
     *
     * @param encoder the current, authoritative password encoder
     */
    public PasswordUpgradeService(PasswordEncoder encoder) {
        this.encoder = encoder;
    }

    /**
     * Result of an authentication attempt.
     *
     * @param ok       {@code true} if the password matched
     * @param upgraded {@code true} if the caller should update the stored hash
     * @param newHash  the new hash to store (non-null only when {@code upgraded=true})
     */
    public record AuthResult(boolean ok, boolean upgraded, String newHash) {

        /** Factory: successful auth, no upgrade needed. */
        static AuthResult success() {
            return new AuthResult(true, false, null);
        }

        /** Factory: successful auth, upgrade needed. */
        static AuthResult successWithUpgrade(String newHash) {
            return new AuthResult(true, true, newHash);
        }

        /** Factory: authentication failed. */
        static AuthResult failure() {
            return new AuthResult(false, false, null);
        }
    }

    /**
     * Authenticates a user by verifying their raw password against the stored hash.
     *
     * <p>If authentication succeeds and the stored hash uses outdated parameters
     * (detected via {@link PasswordEncoder#upgradeEncoding(String)}), this method
     * re-hashes the raw password and returns the new hash in {@link AuthResult#newHash()}.
     *
     * <p><strong>Caller responsibility</strong>: if {@code result.upgraded()} is
     * {@code true}, update the stored hash in the database with {@code result.newHash()}.
     * Failure to do so means the upgrade is lost and the user's hash stays weak.
     *
     * <p><strong>Security note</strong>: the raw password is only held in memory for
     * the duration of this method call. Java's garbage collector doesn't guarantee
     * immediate memory wiping — see "What the toy misses" in the lab for discussion
     * of {@code sodium_memzero()} and secure memory in Go vs Java.
     *
     * @param rawPassword  the plaintext password from the login request
     * @param storedHash   the hash currently stored in the database
     * @return {@link AuthResult} with ok/upgraded/newHash fields
     */
    public AuthResult authenticate(String rawPassword, String storedHash) {
        if (!encoder.matches(rawPassword, storedHash)) {
            return AuthResult.failure();
        }

        // Password matched. Now check if the hash needs upgrading.
        if (encoder.upgradeEncoding(storedHash)) {
            String newHash = encoder.encode(rawPassword);
            return AuthResult.successWithUpgrade(newHash);
        }

        return AuthResult.success();
    }
}
