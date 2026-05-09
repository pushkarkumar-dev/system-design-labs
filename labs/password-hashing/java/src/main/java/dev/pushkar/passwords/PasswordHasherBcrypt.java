package dev.pushkar.passwords;

import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;

/**
 * bcrypt password hashing using Spring Security's {@link BCryptPasswordEncoder}.
 *
 * <p>This is the Java equivalent of the Go {@code HashBcrypt}/{@code VerifyBcrypt} functions.
 * The cost factor (work factor) doubles the computation time for each increment:
 * cost=10 targets ~100ms, cost=14 targets ~1,600ms.
 *
 * <p><strong>Self-describing hash format</strong>: bcrypt embeds the cost and salt in
 * the hash string: {@code $2a$10$<22-char-salt><31-char-hash>}. This means:
 * <ul>
 *   <li>You can verify old hashes even after changing the cost factor.</li>
 *   <li>{@code upgradeEncoding()} returns {@code true} if the stored cost is lower
 *       than the encoder's configured cost — the signal to re-hash on login.</li>
 * </ul>
 *
 * <p><strong>bcrypt weakness</strong>: bcrypt is compute-hard but not memory-hard.
 * Modern GPUs can compute ~10,000 bcrypt hashes/sec (cost=10) in parallel, limited
 * only by compute, not VRAM. Argon2id's memory-hardness limits GPU parallelism to
 * VRAM / memory-per-hash. Prefer Argon2id for new systems (see {@link PasswordHasherArgon2}).
 */
public class PasswordHasherBcrypt {

    private final BCryptPasswordEncoder encoder;

    /**
     * Creates a bcrypt hasher with the default cost (10), targeting ~100ms per hash.
     *
     * <p>Cost=10 is the Spring Security default and the bcrypt standard recommendation.
     * For admin accounts or high-security flows, use cost=12 or cost=14.
     */
    public PasswordHasherBcrypt() {
        this.encoder = new BCryptPasswordEncoder();
    }

    /**
     * Creates a bcrypt hasher with an explicit cost factor.
     *
     * @param cost bcrypt work factor (4–31). Each increment doubles computation time.
     *             Cost=10 → ~100ms. Cost=14 → ~1,600ms. Cost=4 → ~6ms (test only).
     */
    public PasswordHasherBcrypt(int cost) {
        this.encoder = new BCryptPasswordEncoder(cost);
    }

    /**
     * Hashes a raw password with bcrypt.
     *
     * <p>Internally: generates a 16-byte random salt, runs the bcrypt key schedule
     * 2^cost times, and returns the hash in Modular Crypt Format:
     * {@code $2a$<cost>$<22-char-base64-salt><31-char-base64-hash>}
     *
     * @param rawPassword the plaintext password to hash
     * @return bcrypt hash string (60 characters, self-describing)
     */
    public String hash(String rawPassword) {
        return encoder.encode(rawPassword);
    }

    /**
     * Verifies a raw password against a bcrypt hash.
     *
     * <p>Spring Security uses constant-time comparison internally. The verification
     * parses the cost and salt from the stored hash, re-runs bcrypt, and compares
     * using {@code MessageDigest.isEqual()} (constant-time, like Go's {@code subtle.ConstantTimeCompare}).
     *
     * @param rawPassword    the candidate plaintext password
     * @param encodedPassword the stored bcrypt hash
     * @return {@code true} if the password matches
     */
    public boolean verify(String rawPassword, String encodedPassword) {
        return encoder.matches(rawPassword, encodedPassword);
    }

    /**
     * Returns {@code true} if the stored hash was created with a lower cost than
     * this encoder's configured cost — signaling that the password should be
     * re-hashed on the next successful login.
     *
     * <p>This is the Spring Security extension point for transparent hash upgrades.
     * The caller detects the upgrade signal and writes the new hash back to the DB.
     *
     * @param encodedPassword the stored bcrypt hash
     * @return {@code true} if the hash uses an outdated cost factor
     */
    public boolean needsRehash(String encodedPassword) {
        return encoder.upgradeEncoding(encodedPassword);
    }

    /**
     * Returns the underlying {@link PasswordEncoder} for use in Spring Security config.
     */
    public PasswordEncoder asPasswordEncoder() {
        return encoder;
    }
}
