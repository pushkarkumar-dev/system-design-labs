// OAuth 2.0 + OIDC provider demo server.
//
// Routes:
//
//	GET  /authorize                        — Authorization endpoint (v0+)
//	POST /token                            — Token endpoint (v0+)
//	GET  /userinfo                         — UserInfo endpoint (v1+)
//	GET  /.well-known/openid-configuration — OIDC discovery document (v2)
//	GET  /.well-known/jwks.json            — JSON Web Key Set (v2)
//	GET  /login                            — Simulated login page (returns HTML form)
//	POST /login                            — Process login, redirect back to /authorize
//
// Demo flow (run in a browser or with curl):
//
//  1. Visit http://localhost:9000/authorize?client_id=demo-client&
//     redirect_uri=http://localhost:8080/callback&scope=openid+email&
//     response_type=code&state=abc123
//
//  2. You're redirected to /login. Use username=alice, password=password.
//
//  3. After login, you're redirected to the callback with ?code=...&state=abc123
//
//  4. Exchange the code:
//     curl -X POST http://localhost:9000/token \
//     -d "grant_type=authorization_code&code=CODE&client_id=demo-client&
//     client_secret=demo-secret&redirect_uri=http://localhost:8080/callback"
//
//  5. Use the access token:
//     curl -H "Authorization: Bearer ACCESS_TOKEN" http://localhost:9000/userinfo
package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	"dev.pushkar/oauth-provider/pkg/oauth"
)

var provider *oauth.Provider

func main() {
	var err error
	provider, err = oauth.NewProvider(
		[]byte("demo-hmac-secret-32bytes-minimum!"),
		"http://localhost:9000",
	)
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}

	// Register demo clients.
	provider.RegisterClient(&oauth.Client{
		ID:           "demo-client",
		Secret:       "demo-secret",
		RedirectURIs: []string{"http://localhost:8080/callback"},
		Name:         "Demo Confidential App",
		Public:       false,
	})
	provider.RegisterClient(&oauth.Client{
		ID:           "demo-spa",
		RedirectURIs: []string{"http://localhost:3000/callback"},
		Name:         "Demo SPA (PKCE)",
		Public:       true,
	})

	// Register demo users.
	provider.RegisterUser(&oauth.User{
		ID:    "user-1",
		Email: "alice@example.com",
		Name:  "Alice",
	})
	provider.RegisterUser(&oauth.User{
		ID:    "user-2",
		Email: "bob@example.com",
		Name:  "Bob",
	})

	mux := http.NewServeMux()

	// OAuth 2.0 / OIDC endpoints
	mux.HandleFunc("GET /authorize", handleAuthorize)
	mux.HandleFunc("POST /token", handleToken)
	mux.HandleFunc("GET /userinfo", handleUserinfo)
	mux.HandleFunc("POST /userinfo", handleUserinfo) // OIDC allows POST

	// OIDC discovery endpoints
	mux.HandleFunc("GET /.well-known/openid-configuration", handleDiscovery)
	mux.HandleFunc("GET /.well-known/jwks.json", handleJWKS)

	// Simulated login (in production: your real authentication system)
	mux.HandleFunc("GET /login", handleLoginPage)
	mux.HandleFunc("POST /login", handleLoginPost)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := "localhost:9000"
	fmt.Printf("OAuth 2.0 + OIDC provider running at http://%s\n", addr)
	fmt.Printf("\nDemo flow:\n")
	fmt.Printf("  1. Open http://%s/authorize?client_id=demo-client&redirect_uri=http://localhost:8080/callback&scope=openid+email&response_type=code&state=abc123\n", addr)
	fmt.Printf("  2. Login as alice / password\n")
	fmt.Printf("  3. Exchange the code at POST /token\n")
	fmt.Printf("  4. Call GET /userinfo with the access token\n")
	fmt.Printf("\nOIDC discovery: http://%s/.well-known/openid-configuration\n", addr)
	fmt.Printf("JWKS: http://%s/.well-known/jwks.json\n\n", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// ────────────────────────────────────────────────────────────────────────────
// GET /authorize
// ────────────────────────────────────────────────────────────────────────────

// handleAuthorize processes the initial authorization request from a client app.
// In a real provider, the user is authenticated here (via login form, SSO, etc.)
// before the code is issued. We simulate this with a cookie set by /login.
func handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	scope := q.Get("scope")
	state := q.Get("state")
	nonce := q.Get("nonce")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	responseType := q.Get("response_type")

	if responseType != "code" {
		http.Error(w, "unsupported response_type (only 'code' is supported)", http.StatusBadRequest)
		return
	}
	if clientID == "" {
		http.Error(w, "missing client_id", http.StatusBadRequest)
		return
	}

	// Check if the user is logged in (via simulated session cookie).
	userID := getSessionUserID(r)
	if userID == "" {
		// Not logged in — redirect to /login, preserving all authorize params.
		loginURL := "/login?" + r.URL.RawQuery
		http.Redirect(w, r, loginURL, http.StatusFound)
		return
	}

	// User is logged in — issue the authorization code.
	redirectURL, err := provider.Authorize(oauth.AuthorizeParams{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               scope,
		State:               state,
		Nonce:               nonce,
		UserID:              userID,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	})
	if err != nil {
		// In production: render an error page, NOT a redirect (redirect_uri may be invalid)
		http.Error(w, "authorization error: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Redirect the browser to the client's callback URL with the code.
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// ────────────────────────────────────────────────────────────────────────────
// POST /token
// ────────────────────────────────────────────────────────────────────────────

// handleToken processes the token exchange. The client sends the authorization
// code and receives an access token. This is a server-to-server call — the
// browser never sees the access token.
func handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, "invalid_request", "failed to parse form body", http.StatusBadRequest)
		return
	}

	params := oauth.ExchangeParams{
		GrantType:    r.FormValue("grant_type"),
		Code:         r.FormValue("code"),
		ClientID:     r.FormValue("client_id"),
		ClientSecret: r.FormValue("client_secret"),
		RedirectURI:  r.FormValue("redirect_uri"),
		CodeVerifier: r.FormValue("code_verifier"),
	}

	resp, err := provider.Exchange(params)
	if err != nil {
		writeError(w, "invalid_grant", err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // RFC 6749 §5.1: never cache tokens
	w.Header().Set("Pragma", "no-cache")
	json.NewEncoder(w).Encode(resp)
}

// ────────────────────────────────────────────────────────────────────────────
// GET /userinfo
// ────────────────────────────────────────────────────────────────────────────

// handleUserinfo returns identity claims for the authenticated user.
// Protected by Bearer token — the access token from /token.
func handleUserinfo(w http.ResponseWriter, r *http.Request) {
	bearer := extractBearer(r)
	if bearer == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="oauth-provider"`)
		writeError(w, "invalid_token", "missing Bearer token", http.StatusUnauthorized)
		return
	}

	info, err := provider.Userinfo(bearer)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="oauth-provider", error="invalid_token"`)
		writeError(w, "invalid_token", err.Error(), http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// ────────────────────────────────────────────────────────────────────────────
// OIDC discovery and JWKS
// ────────────────────────────────────────────────────────────────────────────

func handleDiscovery(w http.ResponseWriter, r *http.Request) {
	doc := provider.DiscoveryDocument()
	w.Header().Set("Content-Type", "application/json")
	// Cache for 24h — the discovery document rarely changes.
	w.Header().Set("Cache-Control", "max-age=86400")
	json.NewEncoder(w).Encode(doc)
}

func handleJWKS(w http.ResponseWriter, r *http.Request) {
	jwks := provider.JWKS()
	w.Header().Set("Content-Type", "application/json")
	// Cache for 24h — keys rotate infrequently. Clients should re-fetch on unknown kid.
	w.Header().Set("Cache-Control", "max-age=86400")
	json.NewEncoder(w).Encode(jwks)
}

// ────────────────────────────────────────────────────────────────────────────
// Simulated login (not part of OAuth 2.0 spec — it's your auth system)
// ────────────────────────────────────────────────────────────────────────────

// handleLoginPage renders a simple HTML login form.
// In production: this is your real login page (username/password, SSO, MFA, etc.)
func handleLoginPage(w http.ResponseWriter, r *http.Request) {
	authorizeQuery := r.URL.RawQuery
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, loginHTML, html.EscapeString(authorizeQuery))
}

func handleLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	authorizeQuery := r.FormValue("authorize_query")

	// Demo credentials. In production: check against your user service.
	userID := ""
	switch {
	case username == "alice" && password == "password":
		userID = "user-1"
	case username == "bob" && password == "password":
		userID = "user-2"
	default:
		// Login failed — show error
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, loginErrorHTML, html.EscapeString(authorizeQuery))
		return
	}

	// Set session cookie — in production: use a signed/encrypted session token.
	http.SetCookie(w, &http.Cookie{
		Name:     "session_user_id",
		Value:    userID,
		Path:     "/",
		HttpOnly: true,
		// Secure: true, // enable in production (HTTPS only)
		MaxAge: 3600,
	})

	// Redirect back to the authorization endpoint to complete the flow.
	http.Redirect(w, r, "/authorize?"+authorizeQuery, http.StatusFound)
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

func getSessionUserID(r *http.Request) string {
	c, err := r.Cookie("session_user_id")
	if err != nil {
		return ""
	}
	return c.Value
}

func extractBearer(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(authHeader, "Bearer ")
}

func writeError(w http.ResponseWriter, code, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(oauth.ErrorResponse{
		Error:       code,
		Description: description,
	})
}

// ────────────────────────────────────────────────────────────────────────────
// HTML templates (minimal, demo only)
// ────────────────────────────────────────────────────────────────────────────

const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Sign in — OAuth Demo Provider</title>
  <style>
    body { font-family: system-ui, sans-serif; background: #f5f5f5; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
    .card { background: white; border-radius: 8px; padding: 2rem; max-width: 360px; width: 100%%; box-shadow: 0 2px 8px rgba(0,0,0,0.1); }
    h1 { margin: 0 0 1.5rem; font-size: 1.25rem; }
    label { display: block; font-size: 0.9rem; margin-bottom: 0.25rem; color: #444; }
    input[type=text], input[type=password] { width: 100%%; border: 1px solid #ddd; border-radius: 4px; padding: 0.5rem 0.75rem; font-size: 1rem; box-sizing: border-box; margin-bottom: 1rem; }
    button { background: #e8b87a; color: #1a1a1a; border: none; border-radius: 4px; padding: 0.75rem; width: 100%%; font-size: 1rem; cursor: pointer; font-weight: 600; }
    button:hover { background: #d4a46a; }
    .hint { font-size: 0.8rem; color: #888; margin-top: 1rem; }
  </style>
</head>
<body>
<div class="card">
  <h1>OAuth Demo Provider</h1>
  <form method="POST" action="/login">
    <input type="hidden" name="authorize_query" value="%s">
    <label for="username">Username</label>
    <input type="text" id="username" name="username" placeholder="alice" autocomplete="username">
    <label for="password">Password</label>
    <input type="password" id="password" name="password" placeholder="password" autocomplete="current-password">
    <button type="submit">Sign in</button>
  </form>
  <p class="hint">Demo credentials: alice / password &nbsp;|&nbsp; bob / password</p>
</div>
</body>
</html>`

const loginErrorHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><title>Login failed</title></head>
<body>
<h2>Invalid username or password.</h2>
<a href="/login?%s">Try again</a>
</body>
</html>`
