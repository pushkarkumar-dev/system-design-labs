package dev.pushkar.jwt;

import com.nimbusds.jose.jwk.JWKSet;
import com.nimbusds.jose.jwk.RSAKey;
import com.nimbusds.jose.jwk.source.ImmutableJWKSet;
import com.nimbusds.jose.proc.SecurityContext;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.http.SessionCreationPolicy;
import org.springframework.security.oauth2.jwt.JwtDecoder;
import org.springframework.security.oauth2.jwt.NimbusJwtDecoder;
import org.springframework.security.web.SecurityFilterChain;

import java.security.interfaces.RSAPublicKey;
import java.util.Base64;

/**
 * Spring Security 6.x configuration for JWT-based stateless authentication.
 *
 * <p><strong>Key lesson:</strong> Spring Security's {@code oauth2ResourceServer().jwt()}
 * is doing exactly what our {@link JwtVerifier} does — it pins the algorithm at
 * configuration time, not at verification time. {@link NimbusJwtDecoder} never
 * reads the "alg" field from the token to select the verification algorithm.
 * The algorithm is baked into the decoder at construction.
 *
 * <p>Most developers use the simpler YAML-only configuration:
 * <pre>{@code
 * spring:
 *   security:
 *     oauth2:
 *       resourceserver:
 *         jwt:
 *           public-key-location: classpath:public.pem
 * }</pre>
 *
 * <p>This class shows what Spring Security is doing internally when you use that YAML
 * — so you understand the algorithm pinning is already there, even when it's hidden
 * by the abstraction.
 *
 * <p><strong>Why {@code NimbusJwtDecoder.withPublicKey(key)}?</strong>
 * This variant is for single-key RS256 scenarios. When the RS256 public key is
 * known at configuration time (e.g., from an application.yml property), this is
 * the correct approach. For JWKS endpoints (multi-key, rotation-friendly), use
 * {@code NimbusJwtDecoder.withJwkSetUri(uri)} instead.
 */
@Configuration
@EnableWebSecurity
public class SpringSecurityConfig {

    /**
     * The RSA public key, base64-encoded, from application.yml.
     *
     * <p>In production this should be a PEM file loaded via
     * {@code spring.security.oauth2.resourceserver.jwt.public-key-location},
     * not a raw base64 string in application.yml. But this explicit approach
     * makes the key-loading code visible for the lab.
     */
    @Value("${jwt.rsa-public-key-base64:}")
    private String rsaPublicKeyBase64;

    /**
     * Security filter chain — stateless JWT resource server.
     *
     * <p>Key configuration decisions:
     * <ul>
     *   <li>{@code sessionManagement(STATELESS)}: no session cookies. Every request
     *       must carry a valid JWT. This is the correct mode for REST APIs.
     *   <li>{@code csrf().disable()}: safe for stateless APIs because there's no
     *       session to hijack. CSRF protection is only needed for session-based auth.
     *   <li>{@code oauth2ResourceServer().jwt()}: configures the JWT filter chain.
     *       Spring Security will extract the Bearer token, decode it with the
     *       configured {@link JwtDecoder}, and populate the SecurityContext.
     *   <li>The JWKS and health endpoints are public — no auth required.
     *   <li>Everything else requires authentication.
     * </ul>
     */
    @Bean
    public SecurityFilterChain securityFilterChain(HttpSecurity http) throws Exception {
        http
            .sessionManagement(session ->
                session.sessionCreationPolicy(SessionCreationPolicy.STATELESS))
            .csrf(csrf -> csrf.disable()) // safe for stateless JWT APIs
            .authorizeHttpRequests(auth -> auth
                // Public: JWKS endpoint (fetched by verifiers), health check, sign endpoint
                .requestMatchers("/.well-known/jwks.json", "/health", "/sign").permitAll()
                // Everything else requires a valid JWT
                .anyRequest().authenticated()
            )
            .oauth2ResourceServer(oauth2 -> oauth2
                // jwt() wires in the NimbusJwtDecoder bean declared below.
                // The algorithm is pinned in the decoder — it never reads "alg"
                // from the token to decide how to verify.
                .jwt(jwt -> jwt.decoder(jwtDecoder()))
            );

        return http.build();
    }

    /**
     * JWT decoder — algorithm pinned to RS256 at construction time.
     *
     * <p>{@link NimbusJwtDecoder#withPublicKey(RSAPublicKey)} returns a decoder
     * that ONLY accepts RS256 tokens. This is equivalent to our {@link JwtVerifier}
     * constructed as {@code new JwtVerifier(JWSAlgorithm.RS256, publicKey)}.
     *
     * <p>The decoder validates:
     * <ul>
     *   <li>Signature (RS256, with the configured public key)</li>
     *   <li>Algorithm matches RS256 (algorithm confusion prevention)</li>
     *   <li>Expiry ("exp" claim)</li>
     *   <li>Not-before ("nbf" claim, if present)</li>
     * </ul>
     *
     * <p>If you need to accept multiple algorithms or multiple keys (e.g., during
     * rotation), use {@code NimbusJwtDecoder.withJwkSetUri(uri)} with the JWKS
     * endpoint. The JWKS approach is more flexible but still pins algorithms per key.
     */
    @Bean
    public JwtDecoder jwtDecoder() {
        RSAPublicKey publicKey = loadRsaPublicKey();

        // withPublicKey() is the explicit form. Under the hood, it:
        //   1. Wraps the key in a JWKSet (single-key JWKS)
        //   2. Creates a NimbusJwtDecoder with RS256 algorithm pinned
        //   3. The decoder rejects any token where "alg" != "RS256"
        return NimbusJwtDecoder.withPublicKey(publicKey).build();
    }

    /**
     * Load the RSA public key from the application.yml property.
     *
     * <p>In a real application, prefer:
     * <pre>{@code
     * spring:
     *   security:
     *     oauth2:
     *       resourceserver:
     *         jwt:
     *           public-key-location: classpath:keys/public.pem
     * }</pre>
     *
     * <p>That one-liner is equivalent to this code — Spring Security reads the PEM,
     * parses the key, and calls {@code NimbusJwtDecoder.withPublicKey(key).build()}.
     * We show the explicit version so the algorithm-pinning is visible.
     */
    private RSAPublicKey loadRsaPublicKey() {
        if (rsaPublicKeyBase64 == null || rsaPublicKeyBase64.isBlank()) {
            // Fallback for the demo: generate an ephemeral key pair at startup.
            // This means the key changes on every restart — tokens issued before
            // restart are invalid. Fine for demos; not for production.
            try {
                RSAKey key = JwtSignerRS256.generateKeyPair();
                return key.toRSAPublicKey();
            } catch (Exception e) {
                throw new IllegalStateException("Failed to generate demo RSA key", e);
            }
        }

        // Load from the base64-encoded DER bytes in application.yml
        try {
            byte[] keyBytes = Base64.getDecoder().decode(rsaPublicKeyBase64);
            var keySpec = new java.security.spec.X509EncodedKeySpec(keyBytes);
            var keyFactory = java.security.KeyFactory.getInstance("RSA");
            return (RSAPublicKey) keyFactory.generatePublic(keySpec);
        } catch (Exception e) {
            throw new IllegalStateException("Failed to load RSA public key from jwt.rsa-public-key-base64", e);
        }
    }

    /**
     * Optional: expose the RSA public key as a {@link JWKSet} for the JWKS endpoint.
     *
     * <p>If you use Spring Authorization Server, this is handled automatically.
     * For a plain resource server, wire this into a controller at
     * {@code GET /.well-known/jwks.json}.
     *
     * <p>This shows that the JWK format (what verifiers fetch from the JWKS endpoint)
     * and the decoder configuration (what Spring Security uses internally) are
     * representations of the same RSA public key.
     */
    @Bean
    public JWKSet jwkSet() {
        try {
            RSAKey rsaKey = JwtSignerRS256.generateKeyPair();
            // toPublicJWK() strips private key material — safe to serve at JWKS endpoint
            return new JWKSet(rsaKey.toPublicJWK());
        } catch (Exception e) {
            throw new IllegalStateException("Failed to build JWKSet", e);
        }
    }
}
