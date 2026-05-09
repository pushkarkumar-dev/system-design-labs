package dev.pushkar.passwords;

import org.springframework.security.crypto.argon2.Argon2PasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;

/**
 * Argon2id password hashing using Spring Security's {@link Argon2PasswordEncoder}.
 *
 * <p>This is the Java equivalent of the Go {@code HashArgon2id}/{@code VerifyArgon2id}
 * functions. The encoded hash format is the PHC string format:
 * {@code $argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>}
 *
 * <p>This format is identical to our Go implementation — if you hash a password with
 * the Go server and send the encoded string to the Java service, it can verify it
 * (without pepper, since pepper is applied before hashing in both implementations).
 *
 * <p><strong>Memory-hardness explained</strong>: Argon2id requires {@code memory} KiB
 * of RAM per hash. With the default 64MB:
 * <ul>
 *   <li>An RTX 4090 has 24GB VRAM → at most 24,576 / 64 = 384 hashes in parallel.</li>
 *   <li>Compare bcrypt on the same GPU: ~10,000 parallel hashes (only compute-limited).</li>
 *   <li>Argon2id makes GPUs ~26x less effective than bcrypt for brute-force attacks.</li>
 * </ul>
 *
 * <p><strong>OWASP recommendation</strong>: Argon2id with m=65536 (64MB), t=3, p=4
 * is the 2023 OWASP password storage recommendation. These are the defaults used here.
 */
public class PasswordHasherArgon2 {

    private final Argon2PasswordEncoder encoder;

    /**
     * Creates an Argon2id hasher with Spring Security's v5.8+ default parameters:
     * m=16384 (16MB), t=2, p=1, salt=16 bytes, hash=32 bytes.
     *
     * <p>Note: these defaults are more conservative than OWASP's recommendation.
     * Use {@link #withOWASPParams()} for the full OWASP-recommended configuration.
     */
    public PasswordHasherArgon2() {
        this.encoder = Argon2PasswordEncoder.defaultsForSpringSecurity_v5_8();
    }

    /**
     * Creates an Argon2id hasher with OWASP-recommended parameters (2023):
     * m=65536 (64MB), t=3, p=4, salt=16 bytes, hash=32 bytes.
     *
     * <p>These match the Go {@code DefaultArgon2Params()} exactly.
     * Use this for new systems. The Go and Java encoded strings are interoperable.
     *
     * @return a configured {@link PasswordHasherArgon2}
     */
    public static PasswordHasherArgon2 withOWASPParams() {
        return new PasswordHasherArgon2(
                16,     // saltLength bytes
                32,     // hashLength bytes
                2,      // parallelism
                65536,  // memory KiB — 64MB
                3       // iterations
        );
    }

    /**
     * Creates an Argon2id hasher with custom parameters.
     *
     * @param saltLength  salt length in bytes (16 recommended)
     * @param hashLength  output hash length in bytes (32 recommended)
     * @param parallelism number of threads
     * @param memory      memory in KiB (65536 = 64MB recommended)
     * @param iterations  number of passes over memory
     */
    public PasswordHasherArgon2(int saltLength, int hashLength, int parallelism,
                                 int memory, int iterations) {
        this.encoder = new Argon2PasswordEncoder(saltLength, hashLength, parallelism,
                memory, iterations);
    }

    /**
     * Hashes a raw password with Argon2id.
     *
     * <p>The returned string is in PHC format and is self-describing — the parameters
     * are embedded, so verification works even if you later change the default params.
     *
     * @param rawPassword the plaintext password to hash
     * @return Argon2id encoded string: {@code $argon2id$v=19$m=...,t=...,p=...$salt$hash}
     */
    public String hash(String rawPassword) {
        return encoder.encode(rawPassword);
    }

    /**
     * Verifies a raw password against an Argon2id encoded string.
     *
     * <p>Spring Security parses the encoded string, extracts params + salt,
     * recomputes Argon2id, and compares using constant-time byte comparison.
     *
     * @param rawPassword     the candidate plaintext password
     * @param encodedPassword the stored Argon2id encoded string
     * @return {@code true} if the password matches
     */
    public boolean verify(String rawPassword, String encodedPassword) {
        return encoder.matches(rawPassword, encodedPassword);
    }

    /**
     * Returns {@code true} if the stored hash was created with different parameters
     * than this encoder's configuration — signaling that re-hashing is recommended.
     *
     * <p>This is the signal for transparent hash upgrades on login: call
     * {@code verify()}, and if it returns {@code true} and {@code needsRehash()}
     * also returns {@code true}, write a fresh hash back to the database.
     *
     * @param encodedPassword the stored Argon2id encoded string
     * @return {@code true} if the hash parameters differ from this encoder's config
     */
    public boolean needsRehash(String encodedPassword) {
        return encoder.upgradeEncoding(encodedPassword);
    }

    /**
     * Returns the underlying {@link PasswordEncoder} for use in Spring Security config
     * or as a delegate inside {@link PepperingPasswordEncoder}.
     */
    public PasswordEncoder asPasswordEncoder() {
        return encoder;
    }
}
