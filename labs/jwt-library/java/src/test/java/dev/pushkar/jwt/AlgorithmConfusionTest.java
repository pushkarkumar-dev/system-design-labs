package dev.pushkar.jwt;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.jwk.RSAKey;
import com.nimbusds.jwt.JWTClaimsSet;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;

import java.security.interfaces.RSAPublicKey;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/**
 * Algorithm confusion attack test suite.
 *
 * <p>This is the centrepiece test of the JWT lab. It proves two things:
 * <ol>
 *   <li>The algorithm confusion attack is real — a naive verifier (one that reads
 *       "alg" from the token header) accepts a forged HS256 token that was signed
 *       with the RSA public key bytes as the HMAC secret.
 *   <li>The algorithm-pinning fix works — {@link JwtVerifier} (algorithm pinned
 *       at construction) rejects the same forged token because the token's
 *       "alg" doesn't match the pinned algorithm.
 * </ol>
 *
 * <p>Running these tests is the fastest way to convince yourself that the attack
 * and the fix are both correct. Green tests = the implementation matches the spec.
 */
@DisplayName("JWT Algorithm Confusion Attack")
class AlgorithmConfusionTest {

    private RSAKey rsaKey;
    private JwtSignerRS256 signer;
    private RSAPublicKey publicKey;
    private String legitimateToken;

    @BeforeEach
    void setUp() throws Exception {
        rsaKey = JwtSignerRS256.generateKeyPair();
        signer = new JwtSignerRS256(rsaKey);
        publicKey = signer.getPublicKey();

        // A legitimate RS256 token — signed correctly with the private key
        legitimateToken = signer.sign("user:42", "auth.example.com", 1);
    }

    // ── Happy path ────────────────────────────────────────────────────────────

    @Test
    @DisplayName("Safe verifier accepts legitimate RS256 token")
    void legitimateTokenAccepted() throws Exception {
        JwtVerifier safeVerifier = new JwtVerifier(JWSAlgorithm.RS256, publicKey);
        JWTClaimsSet claims = safeVerifier.verify(legitimateToken);

        assertThat(claims.getSubject()).isEqualTo("user:42");
        assertThat(claims.getIssuer()).isEqualTo("auth.example.com");
    }

    // ── The attack ────────────────────────────────────────────────────────────

    @Test
    @DisplayName("ATTACK: naive verifier accepts forged HS256 token (the bug)")
    void naiveVerifierAcceptsForgedToken() throws Exception {
        // Build the forged token: alg=HS256, signed with public key bytes as HMAC secret
        String forgedToken = AlgorithmConfusionDemo.buildForgedToken(legitimateToken, publicKey);

        // The naive verifier reads "alg" from the token header.
        // The forged token says "alg": "HS256".
        // The naive verifier uses the public key DER bytes as the HMAC secret.
        // The attacker signed with the same bytes → verification succeeds.
        byte[] publicKeyBytes = publicKey.getEncoded(); // DER-encoded X.509 public key
        boolean accepted = AlgorithmConfusionDemo.naiveVerify(forgedToken, publicKeyBytes, publicKey);

        assertThat(accepted)
            .as("Naive verifier MUST accept the forged token (demonstrating the vulnerability)")
            .isTrue();
    }

    // ── The fix ───────────────────────────────────────────────────────────────

    @Test
    @DisplayName("FIX: safe verifier rejects forged HS256 token (algorithm mismatch)")
    void safeVerifierRejectsForgedToken() throws Exception {
        String forgedToken = AlgorithmConfusionDemo.buildForgedToken(legitimateToken, publicKey);

        JwtVerifier safeVerifier = new JwtVerifier(JWSAlgorithm.RS256, publicKey);

        // The safe verifier is pinned to RS256.
        // The forged token has "alg": "HS256" in its header.
        // The verifier detects the mismatch and rejects BEFORE signature verification.
        assertThatThrownBy(() -> safeVerifier.verify(forgedToken))
            .isInstanceOf(Exception.class)
            .hasMessageContaining("algorithm mismatch");
    }

    @Test
    @DisplayName("FIX: safe verifier error message names the mismatch explicitly")
    void safeVerifierErrorMessageIsInformative() throws Exception {
        String forgedToken = AlgorithmConfusionDemo.buildForgedToken(legitimateToken, publicKey);
        JwtVerifier safeVerifier = new JwtVerifier(JWSAlgorithm.RS256, publicKey);

        assertThatThrownBy(() -> safeVerifier.verify(forgedToken))
            .hasMessageContaining("HS256")  // the token's alg
            .hasMessageContaining("RS256"); // the pinned alg
    }

    // ── Wrong key ─────────────────────────────────────────────────────────────

    @Test
    @DisplayName("Safe verifier rejects RS256 token signed with a different key")
    void wrongKeyRejected() throws Exception {
        RSAKey otherKey = JwtSignerRS256.generateKeyPair();
        JwtSignerRS256 otherSigner = new JwtSignerRS256(otherKey);
        String tokenFromOtherKey = otherSigner.sign("user:99", "other.example.com", 1);

        // Verify with the original key — should fail
        JwtVerifier verifier = new JwtVerifier(JWSAlgorithm.RS256, publicKey);

        assertThatThrownBy(() -> verifier.verify(tokenFromOtherKey))
            .isInstanceOf(Exception.class)
            .hasMessageContaining("failed");
    }

    // ── HS256 round-trip ──────────────────────────────────────────────────────

    @Test
    @DisplayName("HS256: sign and verify round-trip works correctly")
    void hs256RoundTrip() throws Exception {
        byte[] secret = "this-is-a-32-byte-hmac-secret!!".getBytes();
        JwtSignerHS256 hs256Signer = new JwtSignerHS256(secret);
        JwtVerifier hs256Verifier = new JwtVerifier(JWSAlgorithm.HS256, secret);

        String token = hs256Signer.sign("service:billing", "auth.local", 1);
        JWTClaimsSet claims = hs256Verifier.verify(token);

        assertThat(claims.getSubject()).isEqualTo("service:billing");
    }

    @Test
    @DisplayName("HS256: wrong secret causes verification failure")
    void hs256WrongSecret() throws Exception {
        byte[] correctSecret = "correct-secret-that-is-32-bytes!!".getBytes();
        byte[] wrongSecret   = "wrong-secret-also-32-bytes-long!!".getBytes();

        JwtSignerHS256 hs256Signer = new JwtSignerHS256(correctSecret);
        JwtVerifier hs256Verifier = new JwtVerifier(JWSAlgorithm.HS256, wrongSecret);

        String token = hs256Signer.sign("user:1", "test", 1);

        assertThatThrownBy(() -> hs256Verifier.verify(token))
            .isInstanceOf(Exception.class);
    }

    // ── JWKS round-trip ───────────────────────────────────────────────────────

    @Test
    @DisplayName("JWKS: public key survives JWK serialization and reconstruction")
    void jwksRoundTrip() throws Exception {
        // Simulate: signer publishes JWK at JWKS endpoint; verifier fetches and reconstructs
        RSAKey publicJwk = signer.getPublicKeyJwk(); // what the JWKS endpoint returns
        RSAPublicKey reconstructed = publicJwk.toRSAPublicKey(); // what the verifier loads

        // Verify a token with the reconstructed key
        JwtVerifier verifier = new JwtVerifier(JWSAlgorithm.RS256, reconstructed);
        JWTClaimsSet claims = verifier.verify(legitimateToken);

        assertThat(claims.getSubject()).isEqualTo("user:42");
    }
}
