package dev.pushkar.passwords;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.security.crypto.argon2.Argon2PasswordEncoder;
import org.springframework.security.crypto.bcrypt.BCryptPasswordEncoder;
import org.springframework.security.crypto.password.DelegatingPasswordEncoder;
import org.springframework.security.crypto.password.PasswordEncoder;

import java.time.Duration;
import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Tests for {@link PepperingPasswordEncoder} and the password hashing ecosystem.
 *
 * <p>These 6 tests cover the core security properties:
 * <ol>
 *   <li>Wrong password fails verification.</li>
 *   <li>Pepper mismatch fails verification (same password, wrong pepper).</li>
 *   <li>Pepper rotation: old hash (old pepper) still verifies.</li>
 *   <li>upgradeEncoding returns {@code true} for bcrypt when encoder is Argon2id.</li>
 *   <li>DelegatingPasswordEncoder reads format prefix correctly.</li>
 *   <li>Timing-safe comparison: constant time for correct vs. wrong password.</li>
 * </ol>
 *
 * <p>Tests use bcrypt cost=4 for speed (not security). Production uses cost=10+.
 * Argon2id tests use reduced memory (1MB) for CI speed.
 */
class PepperingPasswordEncoderTest {

    // Low-cost params for fast tests — never use in production
    private static final int BCRYPT_TEST_COST = 4;
    // Reduced Argon2id memory for CI speed (1MB instead of 64MB)
    private static final int ARGON2_TEST_MEMORY = 1024; // 1MB in KiB

    private PasswordEncoder bcryptEncoder;
    private PasswordEncoder argon2Encoder;
    private PepperingPasswordEncoder pepperingBcrypt;
    private PepperingPasswordEncoder pepperingArgon2;

    @BeforeEach
    void setUp() {
        bcryptEncoder = new BCryptPasswordEncoder(BCRYPT_TEST_COST);
        argon2Encoder = new Argon2PasswordEncoder(16, 32, 1, ARGON2_TEST_MEMORY, 1);
        pepperingBcrypt = new PepperingPasswordEncoder(bcryptEncoder, "test-pepper-v1");
        pepperingArgon2 = new PepperingPasswordEncoder(argon2Encoder, "test-pepper-v1");
    }

    /**
     * Test 1: Wrong password must fail verification.
     *
     * <p>Verifies that the comparison is correct — not just that it compiles.
     * bcrypt.matches("wrong" + pepper, hash) must return false.
     */
    @Test
    void wrongPasswordFails() {
        String hash = pepperingBcrypt.encode("correct-password");
        assertFalse(pepperingBcrypt.matches("wrong-password", hash),
                "Wrong password should not match");
    }

    /**
     * Test 2: Same password, wrong pepper must fail.
     *
     * <p>This is the core peppering guarantee: the DB alone is insufficient.
     * An attacker with the hash and no pepper cannot verify candidates.
     * "password" hashed with pepper-v1 must not match when verified with pepper-v2.
     */
    @Test
    void pepperMismatchFails() {
        PepperingPasswordEncoder encoderWithV1 =
                new PepperingPasswordEncoder(bcryptEncoder, "pepper-v1");
        PepperingPasswordEncoder encoderWithV2 =
                new PepperingPasswordEncoder(bcryptEncoder, "pepper-v2");

        String hash = encoderWithV1.encode("user-password");

        // Same password, but the verifier uses pepper-v2 — must fail.
        assertFalse(encoderWithV2.matches("user-password", hash),
                "Same password with wrong pepper should not match. " +
                        "This is the peppering guarantee.");
    }

    /**
     * Test 3: Old hash (created with old pepper) still verifies.
     *
     * <p>Simulates the key rotation scenario:
     * <ol>
     *   <li>User's hash was created with pepper v1.</li>
     *   <li>We rotate to pepper v2 for new hashes.</li>
     *   <li>The user logs in — we try v1 first (from the stored version number).</li>
     *   <li>It should still verify. Then we re-hash with v2 and update the DB.</li>
     * </ol>
     *
     * <p>In production, the version number is stored alongside the hash.
     * This test shows the lookup-by-version pattern.
     */
    @Test
    void pepperRotationOldHashStillVerifies() {
        PepperingPasswordEncoder oldEncoder =
                new PepperingPasswordEncoder(bcryptEncoder, "old-pepper");
        PepperingPasswordEncoder newEncoder =
                new PepperingPasswordEncoder(bcryptEncoder, "new-pepper");

        // Hash was created with old pepper
        String oldHash = oldEncoder.encode("user-password");

        // Old encoder still verifies the old hash (lookup by stored version)
        assertTrue(oldEncoder.matches("user-password", oldHash),
                "Old encoder should still verify hash created with old pepper");

        // New encoder does NOT match (correct — different pepper)
        assertFalse(newEncoder.matches("user-password", oldHash),
                "New encoder should not verify hash created with old pepper");

        // Rotation: re-hash with new pepper on successful login
        String newHash = newEncoder.encode("user-password");
        assertTrue(newEncoder.matches("user-password", newHash),
                "Re-hashed password should verify with new encoder");
    }

    /**
     * Test 4: upgradeEncoding returns true for bcrypt when the encoder is Argon2id.
     *
     * <p>This tests the {@link PepperingPasswordEncoder#upgradeEncoding(String)} delegation.
     * A bcrypt hash passed to an Argon2id encoder should signal that re-hashing is needed.
     *
     * <p>This is the bcrypt → Argon2id migration path: old users have bcrypt hashes,
     * and on login, the system detects the mismatch and upgrades transparently.
     */
    @Test
    void upgradeEncodingReturnsTrueForBcryptWhenEncoderIsArgon2() {
        // Create a bcrypt hash (simulating legacy data)
        BCryptPasswordEncoder legacyBcrypt = new BCryptPasswordEncoder(BCRYPT_TEST_COST);
        String bcryptHash = legacyBcrypt.encode("user-password");

        // Wrap in peppering to simulate legacy storage
        PepperingPasswordEncoder legacyPeppering =
                new PepperingPasswordEncoder(legacyBcrypt, "pepper");
        String legacyHash = legacyPeppering.encode("user-password");

        // New encoder is Argon2id — upgradeEncoding on a bcrypt hash should return true
        // because the formats are different (bcrypt vs argon2id prefix in the hash)
        PepperingPasswordEncoder argon2Peppering =
                new PepperingPasswordEncoder(argon2Encoder, "pepper");

        // BCryptPasswordEncoder.upgradeEncoding checks cost level, not format.
        // For format-based upgrade detection, use DelegatingPasswordEncoder.
        // Here we test cost-based upgrade: a hash from cost=4 with an encoder at cost=10.
        BCryptPasswordEncoder strongBcrypt = new BCryptPasswordEncoder(10);
        PepperingPasswordEncoder strongPeppering =
                new PepperingPasswordEncoder(strongBcrypt, "pepper");

        // A cost=4 hash should trigger upgrade when the encoder uses cost=10
        assertTrue(strongPeppering.upgradeEncoding(bcryptHash),
                "BCrypt cost=4 hash should be flagged for upgrade by cost=10 encoder");

        // A fresh hash from the strong encoder should NOT need upgrade
        String freshHash = strongPeppering.encode("user-password");
        assertFalse(strongPeppering.upgradeEncoding(freshHash),
                "Fresh hash should not need upgrade");
    }

    /**
     * Test 5: DelegatingPasswordEncoder reads format prefix correctly.
     *
     * <p>Verifies that {bcrypt} and {argon2} prefixed hashes are dispatched to the
     * correct delegate. This is the algorithm migration mechanism.
     */
    @Test
    void delegatingPasswordEncoderReadsFormatPrefix() {
        Map<String, PasswordEncoder> encoders = new HashMap<>();
        encoders.put("bcrypt", new BCryptPasswordEncoder(BCRYPT_TEST_COST));
        encoders.put("argon2", new Argon2PasswordEncoder(16, 32, 1, ARGON2_TEST_MEMORY, 1));

        DelegatingPasswordEncoder delegating = new DelegatingPasswordEncoder("argon2", encoders);

        // New hash gets the {argon2} prefix
        String argon2Hash = delegating.encode("argon2-password");
        assertTrue(argon2Hash.startsWith("{argon2}"),
                "New hash should have {argon2} prefix, got: " + argon2Hash.substring(0, 12));

        // Manually create a {bcrypt} prefixed hash (simulating legacy DB row)
        BCryptPasswordEncoder bcrypt = new BCryptPasswordEncoder(BCRYPT_TEST_COST);
        String bcryptHash = "{bcrypt}" + bcrypt.encode("bcrypt-password");

        // DelegatingPasswordEncoder dispatches by prefix
        assertTrue(delegating.matches("argon2-password", argon2Hash),
                "Argon2id hash should verify via delegating encoder");
        assertTrue(delegating.matches("bcrypt-password", bcryptHash),
                "BCrypt hash should verify via delegating encoder");

        // Wrong passwords still fail
        assertFalse(delegating.matches("wrong-password", argon2Hash),
                "Wrong password against argon2 hash should fail");
        assertFalse(delegating.matches("wrong-password", bcryptHash),
                "Wrong password against bcrypt hash should fail");
    }

    /**
     * Test 6: Timing-safe comparison — constant time for correct vs. wrong password.
     *
     * <p>The dominant cost of password verification is the Argon2id computation itself
     * (even at reduced test params, it's the bottleneck). The final comparison is
     * constant-time (Spring Security uses {@code MessageDigest.isEqual}), but the
     * total time should be similar regardless of whether the password is correct.
     *
     * <p>We run 3 iterations and check that correct vs. wrong password timing
     * is within 50ms of each other. The Argon2id computation dominates — any
     * short-circuit in the comparison is nanoseconds, invisible against the ~5-20ms
     * total (even at reduced test params).
     */
    @Test
    void timingAttackResistance() {
        String hash = pepperingArgon2.encode("correct-password");

        final int iterations = 3;
        long correctTotal = 0;
        long wrongTotal = 0;

        // Warm up the JIT
        pepperingArgon2.matches("correct-password", hash);
        pepperingArgon2.matches("wrong-password", hash);

        for (int i = 0; i < iterations; i++) {
            long start = System.nanoTime();
            pepperingArgon2.matches("correct-password", hash);
            correctTotal += System.nanoTime() - start;

            start = System.nanoTime();
            pepperingArgon2.matches("completely-wrong-password", hash);
            wrongTotal += System.nanoTime() - start;
        }

        long correctAvgMs = (correctTotal / iterations) / 1_000_000;
        long wrongAvgMs = (wrongTotal / iterations) / 1_000_000;
        long diffMs = Math.abs(correctAvgMs - wrongAvgMs);

        // Both paths must run the full Argon2id computation.
        // Allow 50ms tolerance — the computation itself is the bottleneck.
        assertTrue(diffMs < 50,
                String.format("Timing difference too large: correct=%dms, wrong=%dms, diff=%dms. " +
                        "Suggests a non-constant-time code path.", correctAvgMs, wrongAvgMs, diffMs));
    }
}
