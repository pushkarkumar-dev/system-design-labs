package dev.pushkar.jwt;

import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSVerifier;
import com.nimbusds.jose.crypto.MACVerifier;
import com.nimbusds.jose.crypto.RSASSAVerifier;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;
import com.nimbusds.jwt.proc.BadJWTException;

import javax.crypto.SecretKey;
import javax.crypto.spec.SecretKeySpec;
import java.security.interfaces.RSAPublicKey;
import java.text.ParseException;
import java.util.Date;

/**
 * Secure JWT verifier — algorithm pinned at construction time.
 *
 * <p><strong>This is the fix to the algorithm confusion attack.</strong>
 *
 * <p>The attack exploits verifiers that read the "alg" field from the token header
 * and use it to select the verification algorithm. Since the header is part of the
 * token, it's attacker-controlled. An attacker can set "alg": "HS256" in any RS256
 * token and sign it with the public key as the HMAC secret.
 *
 * <p>The fix: the algorithm is specified at <em>verifier construction</em>, not
 * read from the token. The token's "alg" field is ignored entirely.
 *
 * <p><strong>Usage patterns:</strong>
 * <pre>{@code
 * // RS256 verifier — algorithm pinned, never read from token
 * JwtVerifier verifier = new JwtVerifier(JWSAlgorithm.RS256, rsaPublicKey);
 *
 * // HS256 verifier — same pattern, symmetric key
 * JwtVerifier verifier = new JwtVerifier(JWSAlgorithm.HS256, hmacSecret);
 * }</pre>
 *
 * <p>This pattern mirrors what Spring Security does internally.
 * {@link org.springframework.security.oauth2.jwt.NimbusJwtDecoder#withPublicKey(RSAPublicKey)}
 * pins the algorithm to RS256. The oauth2ResourceServer() DSL hides this — which
 * is why developers don't realize the fix is already there.
 *
 * <p>Verified properties:
 * <ul>
 *   <li>Signature valid for the pinned algorithm and key</li>
 *   <li>Token is not expired ("exp" claim)</li>
 * </ul>
 * Claims not verified (but should be in production): "iss", "aud", "nbf".
 */
public class JwtVerifier {

    private final JWSAlgorithm pinnedAlgorithm;
    private final JWSVerifier verifier;

    /**
     * RS256 verifier — algorithm pinned to RS256.
     *
     * @param algorithm must be {@link JWSAlgorithm#RS256} (or RS384/RS512)
     * @param publicKey RSA public key — only the public key is needed; verifiers
     *                  never need the private key
     */
    public JwtVerifier(JWSAlgorithm algorithm, RSAPublicKey publicKey) {
        if (!JWSAlgorithm.Family.RSA.contains(algorithm)) {
            throw new IllegalArgumentException(
                "Use the RSA constructor only for RSA algorithms. Got: " + algorithm);
        }
        this.pinnedAlgorithm = algorithm;
        // RSASSAVerifier uses rsa.VerifyPKCS1v15 internally (same as the Go implementation)
        this.verifier = new RSASSAVerifier(publicKey);
    }

    /**
     * HS256 verifier — algorithm pinned to HS256.
     *
     * @param algorithm must be {@link JWSAlgorithm#HS256} (or HS384/HS512)
     * @param secret    the HMAC secret — must be at least 32 bytes
     */
    public JwtVerifier(JWSAlgorithm algorithm, byte[] secret) throws Exception {
        if (!JWSAlgorithm.Family.HMAC_SHA.contains(algorithm)) {
            throw new IllegalArgumentException(
                "Use the HMAC constructor only for HMAC algorithms. Got: " + algorithm);
        }
        this.pinnedAlgorithm = algorithm;
        SecretKey secretKey = new SecretKeySpec(secret, "HmacSHA256");
        this.verifier = new MACVerifier(secretKey);
    }

    /**
     * Verify a compact JWT string.
     *
     * <p>The "alg" field in the token header is checked against the pinned algorithm
     * and <em>rejected</em> if it doesn't match. This is the defence: even if an
     * attacker sets "alg": "HS256" in the header of an RS256 token, this verifier
     * rejects it because the pinned algorithm is RS256.
     *
     * @param token compact JWT (header.payload.signature)
     * @return verified and unexpired claims
     * @throws Exception if the token is malformed, the signature is invalid,
     *                   or the token is expired
     */
    public JWTClaimsSet verify(String token) throws Exception {
        SignedJWT jwt;
        try {
            jwt = SignedJWT.parse(token);
        } catch (ParseException e) {
            throw new Exception("jwt: malformed token — could not parse: " + e.getMessage(), e);
        }

        // ── ALGORITHM PINNING ──────────────────────────────────────────────
        // This is the fix. We compare the token's "alg" against our pinned algorithm.
        // If an attacker changed "alg" from "RS256" to "HS256", this check fails
        // before we even attempt signature verification.
        JWSAlgorithm tokenAlg = jwt.getHeader().getAlgorithm();
        if (!pinnedAlgorithm.equals(tokenAlg)) {
            throw new Exception(String.format(
                "jwt: algorithm mismatch — token has alg=%s but verifier is pinned to %s. " +
                "Possible algorithm confusion attack.",
                tokenAlg, pinnedAlgorithm));
        }

        // ── SIGNATURE VERIFICATION ─────────────────────────────────────────
        // nimbus.verify() returns false on failure (does not throw).
        // MACVerifier and RSASSAVerifier both use constant-time comparison internally.
        boolean valid;
        try {
            valid = jwt.verify(verifier);
        } catch (Exception e) {
            throw new Exception("jwt: signature verification error: " + e.getMessage(), e);
        }
        if (!valid) {
            throw new Exception("jwt: signature verification failed");
        }

        // ── EXPIRY CHECK ───────────────────────────────────────────────────
        JWTClaimsSet claims = jwt.getJWTClaimsSet();
        Date exp = claims.getExpirationTime();
        if (exp != null && exp.before(new Date())) {
            throw new BadJWTException("jwt: token has expired (exp=" + exp + ")");
        }

        return claims;
    }
}
