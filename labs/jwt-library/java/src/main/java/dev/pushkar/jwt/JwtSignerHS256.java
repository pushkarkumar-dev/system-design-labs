package dev.pushkar.jwt;

import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.JWSSigner;
import com.nimbusds.jose.crypto.MACSigner;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;

import javax.crypto.SecretKey;
import javax.crypto.spec.SecretKeySpec;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Date;

/**
 * HS256 JWT signer using nimbus-jose-jwt.
 *
 * <p>This is the Java equivalent of the Go {@code HS256Signer} — same algorithm,
 * same security properties, different API. The goal is to show that a JWT is a
 * standard (RFC 7519) and the implementation details are consistent across
 * languages and libraries.
 *
 * <p><strong>What nimbus does under the hood</strong> (same as our Go implementation):
 * <ol>
 *   <li>Serialize the {@link JWSHeader} to JSON: {@code {"alg":"HS256","typ":"JWT"}}
 *   <li>Serialize the {@link JWTClaimsSet} to JSON.
 *   <li>Base64url-encode both (no padding) → headerB64, payloadB64.
 *   <li>signingInput = headerB64 + "." + payloadB64
 *   <li>signature = HMAC-SHA256(signingInput, secret)
 *   <li>token = signingInput + "." + base64url(signature)
 * </ol>
 *
 * <p><strong>Security note</strong>: nimbus uses constant-time comparison in its
 * verifiers ({@link com.nimbusds.jose.crypto.MACVerifier}). Same as Go's
 * {@code hmac.Equal}. If you write your own verifier, never use
 * {@code String.equals()} on signatures.
 *
 * <p>This class is intentionally minimal — no Spring beans, no annotations.
 * It's a pure JWT utility so you can read it without framework noise.
 */
public class JwtSignerHS256 {

    private final JWSSigner signer;
    private final JWSHeader header;

    /**
     * Creates an HS256 signer.
     *
     * @param secret the HMAC-SHA256 secret — must be at least 32 bytes (256 bits).
     *               Shorter secrets are rejected by nimbus ({@link MACSigner}
     *               enforces the minimum key length from RFC 7518 §3.2).
     */
    public JwtSignerHS256(byte[] secret) throws JOSEException {
        // SecretKeySpec wraps raw bytes into a JCA SecretKey with the "HmacSHA256" algorithm.
        // This is what nimbus expects: a SecretKey, not raw bytes.
        SecretKey secretKey = new SecretKeySpec(secret, "HmacSHA256");

        // MACSigner is nimbus's HMAC signer. It supports HS256, HS384, and HS512.
        // It will throw JOSEException if the key is too short.
        this.signer = new MACSigner(secretKey);

        // Build the header once — it's the same for every token from this signer.
        // JWSHeader.Builder is fluent; .build() produces an immutable JWSHeader.
        this.header = new JWSHeader.Builder(JWSAlgorithm.HS256)
                .type(com.nimbusds.jose.JOSEObjectType.JWT)
                .build();
    }

    /**
     * Sign a JWT with the given subject and expiry.
     *
     * @param subject  the "sub" claim (who the token is about)
     * @param issuer   the "iss" claim (who issued the token)
     * @param ttlHours how long until the token expires (hours)
     * @return compact JWT string: header.payload.signature
     */
    public String sign(String subject, String issuer, int ttlHours) throws JOSEException {
        Instant now = Instant.now();

        // JWTClaimsSet is an immutable, type-safe representation of the payload.
        // nimbus will serialize this to JSON and then base64url-encode it.
        JWTClaimsSet claims = new JWTClaimsSet.Builder()
                .subject(subject)
                .issuer(issuer)
                .issueTime(Date.from(now))
                .expirationTime(Date.from(now.plus(ttlHours, ChronoUnit.HOURS)))
                .build();

        // SignedJWT = the three-part structure: header + payload + (pending) signature.
        // After .sign(), the signature is computed and attached.
        SignedJWT jwt = new SignedJWT(header, claims);
        jwt.sign(signer);  // internals: HMAC-SHA256(header.payload, secret)

        // .serialize() produces the compact representation: header.payload.signature
        // All three parts are base64url-encoded (no padding), dot-separated.
        return jwt.serialize();
    }

    /**
     * Sign a JWT with arbitrary additional claims.
     *
     * @param claimsBuilder a pre-populated builder (caller sets sub, iss, exp, etc.)
     * @return compact JWT string
     */
    public String sign(JWTClaimsSet.Builder claimsBuilder) throws JOSEException {
        SignedJWT jwt = new SignedJWT(header, claimsBuilder.build());
        jwt.sign(signer);
        return jwt.serialize();
    }
}
