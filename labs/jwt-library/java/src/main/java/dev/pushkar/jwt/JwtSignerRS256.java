package dev.pushkar.jwt;

import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.crypto.RSASSASigner;
import com.nimbusds.jose.jwk.RSAKey;
import com.nimbusds.jose.jwk.gen.RSAKeyGenerator;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;

import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Date;
import java.util.UUID;

/**
 * RS256 JWT signer using nimbus-jose-jwt.
 *
 * <p>This is the Java equivalent of the Go {@code RS256Signer} — same algorithm,
 * same security model, different API. Compare the two to see that RSA signing
 * is a standard operation, not a framework-specific abstraction.
 *
 * <p><strong>What nimbus does under the hood</strong> (same as Go):
 * <ol>
 *   <li>Serialize and base64url-encode the JWSHeader + JWTClaimsSet.
 *   <li>Compute SHA-256 hash of the signing input (header.payload).
 *   <li>Sign the hash with the RSA private key using PKCS#1 v1.5 padding.
 *       — This is the expensive step: modular exponentiation with d (~2048 bits).
 *   <li>Base64url-encode the 256-byte RSA signature.
 *   <li>Return: header.payload.signature
 * </ol>
 *
 * <p><strong>Performance reality</strong>: RS256 sign is ~140× slower than HS256
 * sign. The reason is RSA modular exponentiation with a 2048-bit private exponent.
 * This is why OAuth2/OIDC auth servers issue tokens with long TTLs and cache them —
 * signing once per session is fine; signing per request is not.
 *
 * <p><strong>Key generation</strong>: nimbus's {@link RSAKeyGenerator} wraps
 * {@code java.security.KeyPairGenerator}. The generated {@link RSAKey} holds both
 * private and public key material and can export to JWK format for the JWKS endpoint.
 */
public class JwtSignerRS256 {

    private final RSASSASigner signer;
    private final RSAKey publicKeyJwk; // JWK-format public key for JWKS endpoint
    private final String keyId;

    /**
     * Creates an RS256 signer from a nimbus {@link RSAKey} (includes private key).
     *
     * @param rsaKey the full RSA key pair. Must include private key material.
     */
    public JwtSignerRS256(RSAKey rsaKey) throws JOSEException {
        this.keyId = rsaKey.getKeyID();

        // RSASSASigner signs with the private key. Nimbus checks that the key
        // has private key material; it throws JOSEException if it's only a public key.
        RSAPrivateKey privateKey = rsaKey.toRSAPrivateKey();
        this.signer = new RSASSASigner(privateKey);

        // Store only the public key portion for the JWKS endpoint.
        // We never expose the private key outside this class.
        this.publicKeyJwk = rsaKey.toPublicJWK();
    }

    /**
     * Generate a new 2048-bit RSA key pair using nimbus's key generator.
     *
     * <p>In production: load keys from a KMS (AWS KMS, GCP Cloud KMS, HashiCorp Vault).
     * Key generation at startup is fine for demos; it means the key changes on every
     * restart, which invalidates all existing tokens.
     *
     * @return a new RSAKey with a random key ID (UUID)
     */
    public static RSAKey generateKeyPair() throws JOSEException {
        return new RSAKeyGenerator(2048)
                .keyID(UUID.randomUUID().toString())
                .generate();
    }

    /**
     * Sign a JWT with the given subject, issuer, and TTL.
     *
     * <p>The "kid" (key ID) is embedded in the token header so verifiers can look
     * up the right public key from the JWKS endpoint when multiple keys are in use
     * (e.g., during key rotation).
     *
     * @param subject  the "sub" claim
     * @param issuer   the "iss" claim
     * @param ttlHours token lifetime in hours
     * @return compact JWT: header.payload.signature
     */
    public String sign(String subject, String issuer, int ttlHours) throws JOSEException {
        Instant now = Instant.now();

        // Include "kid" in the header so verifiers know which key to fetch from JWKS.
        JWSHeader header = new JWSHeader.Builder(JWSAlgorithm.RS256)
                .type(com.nimbusds.jose.JOSEObjectType.JWT)
                .keyID(keyId)
                .build();

        JWTClaimsSet claims = new JWTClaimsSet.Builder()
                .subject(subject)
                .issuer(issuer)
                .issueTime(Date.from(now))
                .expirationTime(Date.from(now.plus(ttlHours, ChronoUnit.HOURS)))
                .build();

        SignedJWT jwt = new SignedJWT(header, claims);
        jwt.sign(signer); // RSA PKCS#1 v1.5 sign — the expensive step

        return jwt.serialize();
    }

    /**
     * Returns the RSA public key in JWK format for the JWKS endpoint.
     *
     * <p>The JWKS endpoint serves a JSON array of JWK objects. Clients fetch this
     * once, cache it (Cache-Control: max-age=3600), and re-fetch only on unknown "kid".
     *
     * @return RSAKey containing only the public key (no private material)
     */
    public RSAKey getPublicKeyJwk() {
        return publicKeyJwk;
    }

    /**
     * Returns the RSA public key as a standard Java {@link RSAPublicKey}.
     *
     * <p>Use this to construct a {@link JwtVerifier} on the same JVM, or to
     * configure {@link org.springframework.security.oauth2.jwt.NimbusJwtDecoder}.
     */
    public RSAPublicKey getPublicKey() throws JOSEException {
        return publicKeyJwk.toRSAPublicKey();
    }

    /**
     * Returns the key ID embedded in token headers ("kid" field).
     * Used to look up the right JWK from the JWKS endpoint.
     */
    public String getKeyId() {
        return keyId;
    }
}
