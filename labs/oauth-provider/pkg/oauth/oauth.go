// Package oauth is a from-scratch OAuth 2.0 + OIDC provider in three stages.
//
// v0 — Authorization code flow (no PKCE). Client{id, secret}. AuthorizationCode
//
//	single-use with 10-minute expiry. Access tokens are HS256 JWTs. Key lesson:
//	the authorization code flow keeps the access token out of the browser entirely.
//
// v1 — PKCE (Proof Key for Code Exchange). code_challenge / code_verifier per
//
//	RFC 7636. Enables public clients (SPAs, mobile apps) without a client secret.
//	Adds GET /userinfo protected by Bearer token.
//
// v2 — OIDC discovery layer. /.well-known/openid-configuration discovery document.
//
//	GET /.well-known/jwks.json returning the RSA public key set. ID token (JWT with
//	sub, iss, aud, exp, iat, nonce, email) returned alongside access token.
//
// Security lesson: OAuth 2.0 answers "can app X access resource Y on behalf of
// user Z?" OIDC answers "who is user Z?" The ID token is the OIDC answer.
package oauth

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// Shared helpers
// ────────────────────────────────────────────────────────────────────────────

// b64url encodes bytes as base64url with no padding (RFC 4648 §5).
func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeB64url decodes a base64url (no padding) string.
func decodeB64url(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// randHex returns n random bytes encoded as hex — used for authorization codes
// and state tokens. Not base64 to keep URLs clean.
func randBytes(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b64url(b), nil
}

// ────────────────────────────────────────────────────────────────────────────
// v0 — Core types
// ────────────────────────────────────────────────────────────────────────────

// Client represents a registered OAuth 2.0 client application.
// In production, clients are stored in a database and registered via a
// registration API (RFC 7591). Here we hardcode them for clarity.
type Client struct {
	ID           string   // client_id — public identifier
	Secret       string   // client_secret — confidential, never sent in URLs
	RedirectURIs []string // allowed redirect URIs — must match exactly
	Name         string   // human-readable name shown on consent screen
	Public       bool     // if true: PKCE required, secret not checked (v1)
}

// hasRedirectURI checks if the given redirect URI is registered for this client.
// Exact matching is required by the spec (RFC 6749 §10.6). Never match by prefix.
func (c *Client) hasRedirectURI(redirectURI string) bool {
	for _, u := range c.RedirectURIs {
		if u == redirectURI {
			return true
		}
	}
	return false
}

// User represents an authenticated user. In production: looked up in your
// user service after the user logs in via your login page.
type User struct {
	ID    string
	Email string
	Name  string
}

// AuthorizationCode is a short-lived, single-use token that travels via the
// browser redirect. The client exchanges it for an access token server-to-server.
//
// Security properties enforced:
//   - Single-use: deleted on first use. Reuse triggers token revocation (RFC 6819 §4.4.1.1).
//   - 10-minute expiry: short window minimises replay attack opportunity.
//   - Bound to client_id: another client cannot exchange a code issued to client A.
//   - Bound to redirect_uri: the exchange must present the same redirect_uri.
type AuthorizationCode struct {
	Code        string
	ClientID    string
	UserID      string
	Scope       string
	RedirectURI string
	ExpiresAt   time.Time
	Nonce       string // OIDC: opaque value from /authorize, echoed in ID token

	// v1: PKCE fields
	CodeChallenge       string // BASE64URL(SHA256(code_verifier)) sent at /authorize
	CodeChallengeMethod string // "S256" (plain is allowed by spec but discouraged)
}

// TokenResponse is the JSON body returned by POST /token.
// RFC 6749 §5.1.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"` // always "Bearer"
	ExpiresIn   int    `json:"expires_in"` // seconds
	Scope       string `json:"scope,omitempty"`
	IDToken     string `json:"id_token,omitempty"` // v2: OIDC ID token
}

// ErrorResponse is the JSON body returned on OAuth 2.0 errors.
// RFC 6749 §5.2.
type ErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// ────────────────────────────────────────────────────────────────────────────
// v0 — Provider: authorization code flow
// ────────────────────────────────────────────────────────────────────────────

// Provider is the OAuth 2.0 / OIDC server. It holds all runtime state:
// registered clients, in-flight authorization codes, issued tokens, and keys.
//
// Thread safety: all maps are guarded by mu. In production, replace the
// in-memory maps with Redis (for distributed deployments) or PostgreSQL.
type Provider struct {
	mu      sync.Mutex
	clients map[string]*Client        // key: client_id
	users   map[string]*User          // key: user_id (simulates user DB)
	codes   map[string]*AuthorizationCode // key: code

	// v0: HS256 signing key for access tokens (simple, fast)
	hmacSecret []byte

	// v2: RSA key pair for RS256 ID tokens and JWKS endpoint
	rsaKey     *rsa.PrivateKey
	rsaKeyID   string
	issuerURL  string
}

// NewProvider creates a new OAuth provider. hmacSecret must be at least 32 bytes.
// issuerURL is the base URL of this server (e.g., "http://localhost:9000").
func NewProvider(hmacSecret []byte, issuerURL string) (*Provider, error) {
	if len(hmacSecret) < 32 {
		return nil, errors.New("oauth: hmac secret must be at least 32 bytes")
	}

	// Generate RSA key pair for OIDC ID tokens and JWKS endpoint (v2).
	// 2048-bit is the minimum. In production, use at least 2048 or Ed25519.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to generate RSA key: %w", err)
	}

	keyID, err := randBytes(8)
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to generate key ID: %w", err)
	}

	return &Provider{
		clients:   make(map[string]*Client),
		users:     make(map[string]*User),
		codes:     make(map[string]*AuthorizationCode),
		hmacSecret: hmacSecret,
		rsaKey:    rsaKey,
		rsaKeyID:  "key-" + keyID[:8],
		issuerURL: issuerURL,
	}, nil
}

// RegisterClient adds a client application to the provider.
func (p *Provider) RegisterClient(c *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients[c.ID] = c
}

// RegisterUser adds a user to the provider's user store.
func (p *Provider) RegisterUser(u *User) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.users[u.ID] = u
}

// lookupClient finds a client by ID. Returns error if not found.
func (p *Provider) lookupClient(clientID string) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.clients[clientID]
	if !ok {
		return nil, fmt.Errorf("oauth: unknown client_id %q", clientID)
	}
	return c, nil
}

// lookupUser finds a user by ID.
func (p *Provider) lookupUser(userID string) (*User, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	u, ok := p.users[userID]
	if !ok {
		return nil, fmt.Errorf("oauth: unknown user_id %q", userID)
	}
	return u, nil
}

// ────────────────────────────────────────────────────────────────────────────
// v0 — /authorize: issue an authorization code
// ────────────────────────────────────────────────────────────────────────────

// AuthorizeParams holds the validated parameters from GET /authorize.
type AuthorizeParams struct {
	ClientID     string
	RedirectURI  string
	Scope        string
	State        string
	Nonce        string // OIDC
	UserID       string // set by the login layer (not from the request)

	// v1: PKCE
	CodeChallenge       string
	CodeChallengeMethod string
}

// Authorize validates an authorization request and issues an authorization code.
// Returns the redirect URL (with code and state) to send the browser to.
//
// Key lesson: the code travels through the browser redirect (URL parameter).
// The access token NEVER touches the browser. This is the core security
// property of the authorization code flow over the legacy implicit flow.
func (p *Provider) Authorize(params AuthorizeParams) (string, error) {
	// Validate client
	client, err := p.lookupClient(params.ClientID)
	if err != nil {
		return "", err
	}

	// Validate redirect_uri — must match exactly. Never redirect to an
	// unregistered URI; that's an open redirect vulnerability.
	if !client.hasRedirectURI(params.RedirectURI) {
		return "", fmt.Errorf("oauth: redirect_uri %q not registered for client %q", params.RedirectURI, params.ClientID)
	}

	// v1: PKCE validation. Public clients require PKCE (no client_secret).
	// Confidential clients may omit it (though PKCE is recommended for all).
	if client.Public {
		if params.CodeChallenge == "" {
			return "", errors.New("oauth: PKCE required for public clients — include code_challenge")
		}
		if params.CodeChallengeMethod != "S256" {
			return "", errors.New("oauth: only code_challenge_method=S256 is supported")
		}
	}

	// Generate a cryptographically random authorization code.
	// 32 bytes = 256 bits of entropy: effectively unguessable.
	code, err := randBytes(32)
	if err != nil {
		return "", fmt.Errorf("oauth: failed to generate code: %w", err)
	}

	authCode := &AuthorizationCode{
		Code:                code,
		ClientID:            params.ClientID,
		UserID:              params.UserID,
		Scope:               params.Scope,
		RedirectURI:         params.RedirectURI,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
		Nonce:               params.Nonce,
		CodeChallenge:       params.CodeChallenge,
		CodeChallengeMethod: params.CodeChallengeMethod,
	}

	p.mu.Lock()
	p.codes[code] = authCode
	p.mu.Unlock()

	// Build redirect URL: {redirect_uri}?code={code}&state={state}
	redirectURL, err := url.Parse(params.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("oauth: invalid redirect_uri: %w", err)
	}
	q := redirectURL.Query()
	q.Set("code", code)
	if params.State != "" {
		q.Set("state", params.State)
	}
	redirectURL.RawQuery = q.Encode()

	return redirectURL.String(), nil
}

// ────────────────────────────────────────────────────────────────────────────
// v0 — /token: exchange authorization code for access token
// ────────────────────────────────────────────────────────────────────────────

// ExchangeParams holds the validated parameters from POST /token.
type ExchangeParams struct {
	GrantType    string
	Code         string
	ClientID     string
	ClientSecret string
	RedirectURI  string

	// v1: PKCE
	CodeVerifier string
}

// Exchange validates an authorization code and returns an access token.
// The code is consumed (single-use) regardless of success or failure.
//
// Key lesson: if a code is used twice, the spec (RFC 6749 §4.1.2) says to
// revoke all tokens issued from that code — because a replay means the code
// was intercepted. Our provider deletes the code on first use.
func (p *Provider) Exchange(params ExchangeParams) (*TokenResponse, error) {
	if params.GrantType != "authorization_code" {
		return nil, fmt.Errorf("oauth: unsupported grant_type %q", params.GrantType)
	}

	// Look up and immediately delete the code (single-use enforcement).
	p.mu.Lock()
	authCode, ok := p.codes[params.Code]
	if ok {
		delete(p.codes, params.Code)
	}
	p.mu.Unlock()

	if !ok {
		return nil, errors.New("oauth: invalid or already-used authorization_code")
	}

	// Check expiry. Codes are valid for 10 minutes from issuance.
	if time.Now().After(authCode.ExpiresAt) {
		return nil, errors.New("oauth: authorization_code has expired")
	}

	// Validate client_id binding — another client cannot steal a code.
	if authCode.ClientID != params.ClientID {
		return nil, errors.New("oauth: code was not issued to this client_id")
	}

	// Validate redirect_uri — must match the /authorize request exactly.
	// This prevents authorization code injection (RFC 6749 §10.6).
	if authCode.RedirectURI != params.RedirectURI {
		return nil, errors.New("oauth: redirect_uri does not match the authorization request")
	}

	// Validate client credentials (confidential clients only).
	client, err := p.lookupClient(params.ClientID)
	if err != nil {
		return nil, err
	}
	if !client.Public {
		if client.Secret != params.ClientSecret {
			return nil, errors.New("oauth: invalid client_secret")
		}
	}

	// v1: PKCE verification.
	// Verify: BASE64URL(SHA256(code_verifier)) == code_challenge
	// This proves the same party that sent /authorize is now sending /token.
	if authCode.CodeChallenge != "" {
		if params.CodeVerifier == "" {
			return nil, errors.New("oauth: code_verifier required (PKCE was used in authorization request)")
		}
		digest := sha256.Sum256([]byte(params.CodeVerifier))
		computed := b64url(digest[:])
		if computed != authCode.CodeChallenge {
			return nil, errors.New("oauth: code_verifier does not match code_challenge (PKCE verification failed)")
		}
	}

	// Issue access token (HS256 JWT for simplicity; use RS256 for multi-service).
	accessToken, err := p.issueAccessToken(authCode.UserID, authCode.Scope, authCode.ClientID)
	if err != nil {
		return nil, fmt.Errorf("oauth: failed to issue access token: %w", err)
	}

	resp := &TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600, // 1 hour
		Scope:       authCode.Scope,
	}

	// v2: issue ID token if "openid" scope was requested.
	if containsScope(authCode.Scope, "openid") {
		idToken, err := p.issueIDToken(authCode.UserID, authCode.ClientID, authCode.Nonce)
		if err != nil {
			return nil, fmt.Errorf("oauth: failed to issue ID token: %w", err)
		}
		resp.IDToken = idToken
	}

	return resp, nil
}

// ────────────────────────────────────────────────────────────────────────────
// v0 — JWT: HS256 access token issuance and verification
// ────────────────────────────────────────────────────────────────────────────

// issueAccessToken creates a signed HS256 JWT for the given user.
// Claims: sub, iss, aud, iat, exp, scope.
func (p *Provider) issueAccessToken(userID, scope, clientID string) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"sub":   userID,
		"iss":   p.issuerURL,
		"aud":   clientID,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"scope": scope,
	}

	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := b64url(headerJSON) + "." + b64url(payloadJSON)
	mac := hmac.New(sha256.New, p.hmacSecret)
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)

	return signingInput + "." + b64url(sig), nil
}

// VerifyAccessToken validates a Bearer access token and returns its claims.
// Used by the /userinfo endpoint and resource servers.
func (p *Provider) VerifyAccessToken(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("oauth: malformed token")
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, p.hmacSecret)
	mac.Write([]byte(signingInput))
	expected := mac.Sum(nil)

	provided, err := decodeB64url(parts[2])
	if err != nil {
		return nil, errors.New("oauth: invalid token signature encoding")
	}

	// Constant-time comparison prevents timing attacks.
	if !hmac.Equal(expected, provided) {
		return nil, errors.New("oauth: invalid token signature")
	}

	payloadBytes, err := decodeB64url(parts[1])
	if err != nil {
		return nil, errors.New("oauth: invalid token payload encoding")
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("oauth: invalid token payload")
	}

	// Check expiry.
	if expRaw, ok := claims["exp"]; ok {
		if expF, ok := expRaw.(float64); ok {
			if time.Now().Unix() > int64(expF) {
				return nil, errors.New("oauth: token has expired")
			}
		}
	}

	return claims, nil
}

// ────────────────────────────────────────────────────────────────────────────
// v1 — /userinfo: return claims for the authenticated user
// ────────────────────────────────────────────────────────────────────────────

// UserinfoResponse is the JSON body returned by GET /userinfo.
// Standard OIDC UserInfo claims (https://openid.net/specs/openid-connect-core-1_0.html §5.1).
type UserinfoResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Userinfo returns user identity claims for a valid Bearer access token.
// The token must have been issued for a user in this provider's user store.
func (p *Provider) Userinfo(bearerToken string) (*UserinfoResponse, error) {
	claims, err := p.VerifyAccessToken(bearerToken)
	if err != nil {
		return nil, err
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, errors.New("oauth: token missing sub claim")
	}

	user, err := p.lookupUser(sub)
	if err != nil {
		// sub does not correspond to a known user — token was issued for a
		// deleted or unknown user.
		return nil, fmt.Errorf("oauth: user not found for sub %q", sub)
	}

	return &UserinfoResponse{
		Sub:   user.ID,
		Email: user.Email,
		Name:  user.Name,
	}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// v2 — OIDC: ID token issuance (RS256 JWT)
// ────────────────────────────────────────────────────────────────────────────

// issueIDToken creates an RS256-signed OIDC ID token.
// ID token claims (OIDC Core §2): sub, iss, aud, exp, iat, nonce, email.
//
// The ID token proves WHO the user is (identity).
// The access token proves WHAT the user authorized (permission scope).
// They are separate tokens with separate purposes.
func (p *Provider) issueIDToken(userID, clientID, nonce string) (string, error) {
	user, err := p.lookupUser(userID)
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := map[string]any{
		"sub": userID,
		"iss": p.issuerURL,
		"aud": clientID,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
		// email claim — included because "email" scope was requested
		"email": user.Email,
		"name":  user.Name,
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}

	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": p.rsaKeyID,
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	signingInput := b64url(headerJSON) + "." + b64url(payloadJSON)
	digest := sha256.Sum256([]byte(signingInput))

	sigBytes, err := rsa.SignPKCS1v15(rand.Reader, p.rsaKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("oauth: RSA sign failed: %w", err)
	}

	return signingInput + "." + b64url(sigBytes), nil
}

// ────────────────────────────────────────────────────────────────────────────
// v2 — OIDC discovery: /.well-known/openid-configuration
// ────────────────────────────────────────────────────────────────────────────

// OIDCDiscovery is the response body for /.well-known/openid-configuration.
// Defined by OIDC Discovery 1.0 §3.
// Clients use this to discover endpoints without hardcoding URLs — essential
// for supporting multiple environments (dev/staging/prod) from the same config.
type OIDCDiscovery struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	UserinfoEndpoint                 string   `json:"userinfo_endpoint"`
	JWKsURI                          string   `json:"jwks_uri"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
}

// DiscoveryDocument returns the OIDC discovery document for this provider.
// Clients (like Spring Security OAuth2 Client) fetch this once at startup
// and cache the endpoint URLs and signing algorithm preferences.
func (p *Provider) DiscoveryDocument() *OIDCDiscovery {
	base := p.issuerURL
	return &OIDCDiscovery{
		Issuer:                           base,
		AuthorizationEndpoint:            base + "/authorize",
		TokenEndpoint:                    base + "/token",
		UserinfoEndpoint:                 base + "/userinfo",
		JWKsURI:                          base + "/.well-known/jwks.json",
		ResponseTypesSupported:           []string{"code"},
		SubjectTypesSupported:            []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ScopesSupported:                  []string{"openid", "email", "profile"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "none"},
		ClaimsSupported:                  []string{"sub", "iss", "aud", "exp", "iat", "email", "name", "nonce"},
		CodeChallengeMethodsSupported:    []string{"S256"},
	}
}

// ────────────────────────────────────────────────────────────────────────────
// v2 — JWKS endpoint: /.well-known/jwks.json
// ────────────────────────────────────────────────────────────────────────────

// JWK is a single JSON Web Key (RFC 7517) for RSA public keys.
type JWK struct {
	Kty string `json:"kty"` // "RSA"
	Use string `json:"use"` // "sig"
	Kid string `json:"kid"` // key ID matching token header "kid"
	Alg string `json:"alg"` // "RS256"
	N   string `json:"n"`   // modulus (base64url)
	E   string `json:"e"`   // public exponent (base64url)
}

// JWKSet is the full JWKS document.
type JWKSet struct {
	Keys []JWK `json:"keys"`
}

// JWKS returns the public key set. Clients use this to verify ID token
// signatures without sharing a secret. The RSA private key never leaves
// the server; the public key is freely distributed.
//
// Key rotation: when you rotate keys, keep the old key in the JWKS until
// all tokens signed with it have expired. Clients should cache the JWKS
// and re-fetch on unknown "kid" (cache-and-refresh pattern).
func (p *Provider) JWKS() *JWKSet {
	pub := &p.rsaKey.PublicKey
	nBytes := pub.N.Bytes()
	eBig := big.NewInt(int64(pub.E))
	eBytes := eBig.Bytes()

	return &JWKSet{
		Keys: []JWK{{
			Kty: "RSA",
			Use: "sig",
			Kid: p.rsaKeyID,
			Alg: "RS256",
			N:   b64url(nBytes),
			E:   b64url(eBytes),
		}},
	}
}

// RSAPublicKey returns the RSA public key for testing / key export.
func (p *Provider) RSAPublicKey() *rsa.PublicKey {
	return &p.rsaKey.PublicKey
}

// RSAKeyID returns the key ID for the current signing key.
func (p *Provider) RSAKeyID() string {
	return p.rsaKeyID
}

// VerifyIDToken verifies an RS256 ID token signature and expiry.
// Used in tests and by resource servers that need to inspect identity.
func (p *Provider) VerifyIDToken(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("oauth: malformed ID token")
	}

	// Verify RSA signature
	signingInput := parts[0] + "." + parts[1]
	digest := sha256.Sum256([]byte(signingInput))

	sigBytes, err := decodeB64url(parts[2])
	if err != nil {
		return nil, errors.New("oauth: invalid ID token signature encoding")
	}

	if err := rsa.VerifyPKCS1v15(&p.rsaKey.PublicKey, crypto.SHA256, digest[:], sigBytes); err != nil {
		return nil, fmt.Errorf("oauth: ID token signature verification failed: %w", err)
	}

	payloadBytes, err := decodeB64url(parts[1])
	if err != nil {
		return nil, errors.New("oauth: invalid ID token payload")
	}

	var claims map[string]any
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("oauth: invalid ID token payload JSON")
	}

	// Check expiry
	if expRaw, ok := claims["exp"]; ok {
		if expF, ok := expRaw.(float64); ok {
			if time.Now().Unix() > int64(expF) {
				return nil, errors.New("oauth: ID token has expired")
			}
		}
	}

	return claims, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Public key export (for testing / JWKS verification)
// ────────────────────────────────────────────────────────────────────────────

// PublicKeyDER returns the DER-encoded (PKIX/X.509) RSA public key bytes.
// Used in tests to verify that the JWKS endpoint returns a valid key.
func (p *Provider) PublicKeyDER() ([]byte, error) {
	return x509.MarshalPKIXPublicKey(&p.rsaKey.PublicKey)
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

// containsScope checks if a space-separated scope string contains a given scope.
func containsScope(scopes, target string) bool {
	for _, s := range strings.Fields(scopes) {
		if s == target {
			return true
		}
	}
	return false
}
