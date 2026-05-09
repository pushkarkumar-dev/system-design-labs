package dev.pushkar.passwords;

import org.springframework.security.crypto.password.PasswordEncoder;

/**
 * A custom Spring Security {@link PasswordEncoder} that wraps another encoder
 * (Argon2id or bcrypt) and adds a server-side pepper.
 *
 * <p>A pepper is a server-side secret that is NOT stored in the database.
 * It is appended to every password before delegating to the underlying encoder.
 * The hash stored in the DB is: {@code encoder.encode(rawPassword + pepper)}.
 *
 * <p><strong>Why peppering matters</strong>: If the database is stolen, an attacker
 * must also obtain the pepper to crack any password. Without it, the stored hashes
 * are worthless — the attacker is hashing candidates against a target that includes
 * a secret they don't know. This is defense-in-depth: the DB alone is not enough.
 *
 * <p><strong>Where peppers live</strong>: Environment variables, AWS Secrets Manager,
 * HashiCorp Vault, or an HSM. Never in the database. Never in version control.
 *
 * <p><strong>Comparison with salts</strong>: Salts prevent rainbow tables and ensure
 * two users with the same password have different hashes. Peppers add a server-side
 * secret that cannot be obtained by DB exfiltration alone. They are complementary.
 *
 * <p>This class implements the Spring Security {@link PasswordEncoder} interface,
 * so it can be used as a drop-in replacement anywhere a {@link PasswordEncoder} is
 * expected — including {@link org.springframework.security.config.annotation.web.configuration.EnableWebSecurity}
 * configuration and {@link PasswordUpgradeService}.
 */
public class PepperingPasswordEncoder implements PasswordEncoder {

    private final PasswordEncoder delegate;
    private final String pepper;

    /**
     * Creates a {@link PepperingPasswordEncoder} wrapping the given encoder.
     *
     * @param delegate the underlying password encoder (Argon2id or bcrypt)
     * @param pepper   the server-side secret to append before hashing.
     *                 Load from environment variables or a secrets manager,
     *                 never hard-code in source.
     */
    public PepperingPasswordEncoder(PasswordEncoder delegate, String pepper) {
        if (delegate == null) throw new IllegalArgumentException("delegate must not be null");
        if (pepper == null || pepper.isEmpty()) throw new IllegalArgumentException("pepper must not be empty");
        this.delegate = delegate;
        this.pepper = pepper;
    }

    /**
     * Hashes {@code rawPassword + pepper} using the delegate encoder.
     *
     * <p>The pepper is appended (not prepended) to avoid length extension issues
     * with naive hash constructions. Since we're using bcrypt or Argon2id (not a
     * plain hash), this distinction is academic — both are collision-resistant and
     * designed for password hashing.
     *
     * @param rawPassword the plaintext password
     * @return encoded hash of (rawPassword + pepper), using the delegate algorithm
     */
    @Override
    public String encode(CharSequence rawPassword) {
        return delegate.encode(rawPassword.toString() + pepper);
    }

    /**
     * Verifies {@code rawPassword + pepper} against the stored encoded password.
     *
     * <p>Delegates to the underlying encoder's {@code matches()} method, which
     * uses constant-time comparison. A wrong password takes the same time as a
     * correct one — the Argon2id/bcrypt computation dominates.
     *
     * @param rawPassword     the candidate plaintext password
     * @param encodedPassword the stored hash
     * @return {@code true} if (rawPassword + pepper) matches the stored hash
     */
    @Override
    public boolean matches(CharSequence rawPassword, String encodedPassword) {
        return delegate.matches(rawPassword.toString() + pepper, encodedPassword);
    }

    /**
     * Returns {@code true} if the stored hash should be upgraded.
     *
     * <p>Delegates to the underlying encoder. For bcrypt, this returns {@code true}
     * if the stored cost is lower than the encoder's configured cost. For Argon2id,
     * it returns {@code true} if the stored parameters differ from the encoder's config.
     *
     * <p>This is the hook for transparent hash upgrades: check this after a successful
     * {@link #matches(CharSequence, String)} call and re-hash if needed.
     *
     * @param encodedPassword the stored hash
     * @return {@code true} if the hash should be upgraded
     */
    @Override
    public boolean upgradeEncoding(String encodedPassword) {
        return delegate.upgradeEncoding(encodedPassword);
    }
}
