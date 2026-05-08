package jwt_test

import (
	"crypto/x509"
	"strings"
	"testing"
	"time"

	"dev.pushkar/jwt-library/pkg/jwt"
)

// ────────────────────────────────────────────────────────────────────────────
// v0: HS256 round-trip
// ────────────────────────────────────────────────────────────────────────────

func TestRoundTripHS256(t *testing.T) {
	secret := []byte("super-secret-key-that-is-at-least-32-bytes-long")
	signer, err := jwt.NewHS256Signer(secret)
	if err != nil {
		t.Fatalf("NewHS256Signer: %v", err)
	}
	verifier := jwt.NewHS256Verifier(secret)

	claims := jwt.StandardClaims("user:42", "test-issuer", time.Hour, jwt.Claims{
		"role": "admin",
		"org":  "acme",
	})

	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Token must be three dot-separated parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3-part token, got %d parts: %s", len(parts), token)
	}

	got, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if got["sub"] != "user:42" {
		t.Errorf("sub: got %q, want %q", got["sub"], "user:42")
	}
	if got["role"] != "admin" {
		t.Errorf("role: got %q, want %q", got["role"], "admin")
	}
}

func TestHS256WrongSecret(t *testing.T) {
	signer, _ := jwt.NewHS256Signer([]byte("correct-secret-that-is-at-least-32-bytes"))
	wrongVerifier := jwt.NewHS256Verifier([]byte("wrong-secret-that-is-also-long-enough!!"))

	token, _ := signer.Sign(jwt.Claims{"sub": "user:1"})

	_, err := wrongVerifier.Verify(token)
	if err == nil {
		t.Fatal("expected verification failure with wrong secret, got nil error")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// v0: Expired token
// ────────────────────────────────────────────────────────────────────────────

func TestExpiredToken(t *testing.T) {
	secret := []byte("some-secret-key-that-is-at-least-32-bytes-long!!")
	signer, _ := jwt.NewHS256Signer(secret)
	verifier := jwt.NewHS256Verifier(secret)

	// Sign a token that expired 5 seconds ago
	claims := jwt.Claims{
		"sub": "user:99",
		"exp": float64(time.Now().Add(-5 * time.Second).Unix()),
	}
	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = verifier.Verify(token)
	if err == nil {
		t.Fatal("expected expiry error, got nil")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("expected 'expired' in error, got: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// v0: Modified payload must fail verification
// ────────────────────────────────────────────────────────────────────────────

func TestModifiedPayload(t *testing.T) {
	secret := []byte("tamper-test-secret-key-at-least-32-bytes-long!")
	signer, _ := jwt.NewHS256Signer(secret)
	verifier := jwt.NewHS256Verifier(secret)

	claims := jwt.Claims{"sub": "user:1", "role": "user"}
	token, _ := signer.Sign(claims)

	// The payload is the middle part. Replace it with a modified claims
	// (e.g., escalating role from "user" to "admin").
	// The signature was computed over the original payload, so the new
	// payload will have a different HMAC — verification must fail.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal("expected 3-part token")
	}

	// Craft a payload with admin role (base64url-encoded JSON)
	// We just flip a character in the existing payload to simulate tampering.
	originalPayload := parts[1]
	tamperedPayload := originalPayload[:len(originalPayload)-1] + "X"
	tamperedToken := parts[0] + "." + tamperedPayload + "." + parts[2]

	_, err := verifier.Verify(tamperedToken)
	if err == nil {
		t.Fatal("expected verification failure for tampered payload, got nil error")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// v1: RS256 round-trip
// ────────────────────────────────────────────────────────────────────────────

func TestRoundTripRS256(t *testing.T) {
	privKey, err := jwt.GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}

	signer := jwt.NewRS256Signer(privKey, "key-2024-01")
	verifier := jwt.NewRS256Verifier(&privKey.PublicKey)

	claims := jwt.StandardClaims("service:billing", "auth.example.com", time.Hour, jwt.Claims{
		"scope": []string{"read", "write"},
	})

	token, err := signer.Sign(claims)
	if err != nil {
		t.Fatalf("RS256 Sign: %v", err)
	}

	got, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("RS256 Verify: %v", err)
	}

	if got["sub"] != "service:billing" {
		t.Errorf("sub: got %q, want %q", got["sub"], "service:billing")
	}
}

// TestRS256WrongKey ensures a token signed with key A fails verification with key B.
func TestRS256WrongKey(t *testing.T) {
	keyA, _ := jwt.GenerateRSAKeyPair()
	keyB, _ := jwt.GenerateRSAKeyPair()

	signer := jwt.NewRS256Signer(keyA, "key-A")
	wrongVerifier := jwt.NewRS256Verifier(&keyB.PublicKey)

	token, _ := signer.Sign(jwt.Claims{"sub": "user:1"})

	_, err := wrongVerifier.Verify(token)
	if err == nil {
		t.Fatal("expected verification failure with wrong RSA key, got nil error")
	}
}

// ────────────────────────────────────────────────────────────────────────────
// v2: Algorithm confusion attack
// ────────────────────────────────────────────────────────────────────────────
//
// This test is the centrepiece of the lab. It:
//   1. Creates a legitimate RS256 token.
//   2. Forges a new token using the attack (alg=HS256, public key as HMAC secret).
//   3. Shows the NAIVE verifier accepts the forged token (the attack works).
//   4. Shows the SAFE verifier (algorithm pinned at construction) rejects it.

func TestAlgorithmConfusionAttack(t *testing.T) {
	privKey, err := jwt.GenerateRSAKeyPair()
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair: %v", err)
	}
	pubKey := &privKey.PublicKey

	// Step 1: Create a legitimate RS256 token
	signer := jwt.NewRS256Signer(privKey, "prod-key-01")
	legitimateClaims := jwt.Claims{
		"sub":  "user:1",
		"role": "user",
		"exp":  float64(time.Now().Add(time.Hour).Unix()),
	}
	legitimateToken, err := signer.Sign(legitimateClaims)
	if err != nil {
		t.Fatalf("RS256 Sign: %v", err)
	}

	// Step 2: Build the forged token using the algorithm confusion attack
	// The attacker uses the RS256 public key DER bytes as the HMAC-SHA256 secret.
	forgedToken, err := jwt.ConfusionAttackToken(legitimateToken, pubKey)
	if err != nil {
		t.Fatalf("ConfusionAttackToken: %v", err)
	}

	// The forged token must look different from the original
	if forgedToken == legitimateToken {
		t.Fatal("forged token is identical to original — attack construction failed")
	}

	// Step 3: NAIVE verifier (reads alg from header) — MUST ACCEPT the forged token.
	// The naive verifier uses the public key DER bytes as its HMAC secret, which is
	// exactly what ConfusionAttackToken used to sign. So it accepts.
	pubKeyDER, err := x509.MarshalPKIXPublicKey(pubKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	naiveVerifier := jwt.NewNaiveVerifier(pubKeyDER, pubKey)

	_, naiveErr := naiveVerifier.Verify(forgedToken)
	if naiveErr != nil {
		// The attack failed — this is unexpected for a naive verifier
		t.Fatalf("ATTACK FAILED (unexpected): naive verifier rejected the forged token: %v\n"+
			"This means the naive verifier is actually safe — investigate ConfusionAttackToken.", naiveErr)
	}
	t.Logf("ATTACK SUCCEEDED (expected): naive verifier accepted the forged token — the bug is real")

	// Step 4: SAFE verifier (algorithm pinned to RS256) — MUST REJECT the forged token.
	// The safe verifier ignores "alg" from the token and always uses RS256 verification.
	// The forged token's signature is an HMAC, not an RSA signature — RSA verify fails.
	safeVerifier := jwt.NewRS256Verifier(pubKey)
	_, safeErr := safeVerifier.Verify(forgedToken)
	if safeErr == nil {
		t.Fatal("SAFE VERIFIER ACCEPTED FORGED TOKEN — the algorithm-pinning fix is broken")
	}
	t.Logf("SAFE VERIFIER REJECTED FORGED TOKEN (expected): %v", safeErr)
}

// TestAlgorithmConfusionLegitimateToken ensures the safe RS256 verifier still
// accepts a legitimately-signed RS256 token (the fix doesn't break normal use).
func TestAlgorithmConfusionLegitimateToken(t *testing.T) {
	privKey, _ := jwt.GenerateRSAKeyPair()
	signer := jwt.NewRS256Signer(privKey, "key-01")
	verifier := jwt.NewRS256Verifier(&privKey.PublicKey)

	token, _ := signer.Sign(jwt.Claims{
		"sub": "user:42",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})

	_, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("safe verifier rejected a legitimate RS256 token: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// v1: JWKS round-trip
// ────────────────────────────────────────────────────────────────────────────

func TestJWKSRoundTrip(t *testing.T) {
	privKey, _ := jwt.GenerateRSAKeyPair()
	pubKey := &privKey.PublicKey

	// Convert to JWK format (what the JWKS endpoint serves)
	jwk := jwt.PublicKeyToJWK(pubKey, "key-abc")

	// Convert back to *rsa.PublicKey (what verifiers use after fetching JWKS)
	reconstructed, err := jwt.JWKToPublicKey(jwk)
	if err != nil {
		t.Fatalf("JWKToPublicKey: %v", err)
	}

	// Verify a token with the reconstructed key to prove it's byte-for-byte identical
	signer := jwt.NewRS256Signer(privKey, "key-abc")
	verifier := jwt.NewRS256Verifier(reconstructed)

	token, _ := signer.Sign(jwt.Claims{
		"sub": "test",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})

	if _, err := verifier.Verify(token); err != nil {
		t.Fatalf("verification with JWK-reconstructed key failed: %v", err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Security edge cases
// ────────────────────────────────────────────────────────────────────────────

func TestHS256SecretTooShort(t *testing.T) {
	_, err := jwt.NewHS256Signer([]byte("short"))
	if err == nil {
		t.Fatal("expected error for short secret, got nil")
	}
}

func TestMalformedToken(t *testing.T) {
	verifier := jwt.NewHS256Verifier([]byte("secret-key-that-is-at-least-32-bytes-long!!!"))

	for _, bad := range []string{
		"",
		"notavalidtoken",
		"only.two",
		"a.b.c.d", // four parts
	} {
		_, err := verifier.Verify(bad)
		if err == nil {
			t.Errorf("expected error for malformed token %q, got nil", bad)
		}
	}
}
