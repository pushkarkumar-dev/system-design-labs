// JWT library HTTP server — demonstrates sign, verify, and JWKS endpoints.
//
// Routes:
//
//	POST /sign        — sign claims, returns compact JWT
//	POST /verify      — verify a token, returns claims or error
//	GET  /.well-known/jwks.json — public keys in JWKS format
//	GET  /health      — liveness check
//
// The server starts with both HS256 and RS256 keys. Clients choose via
// {"alg": "HS256"} or {"alg": "RS256"} in the /sign request body.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"dev.pushkar/jwt-library/pkg/jwt"
)

// ────────────────────────────────────────────────────────────────────────────
// Server state
// ────────────────────────────────────────────────────────────────────────────

type server struct {
	hs256Signer   *jwt.HS256Signer
	hs256Verifier *jwt.HS256Verifier
	rs256Signer   *jwt.RS256Signer
	rs256Verifier *jwt.RS256Verifier
	jwks          jwt.JWKSKeySet
}

func newServer() (*server, error) {
	// HS256 key — in production, load from a secrets manager
	hmacSecret := []byte("demo-hs256-secret-change-this-before-production-use!")
	hs256Signer, err := jwt.NewHS256Signer(hmacSecret)
	if err != nil {
		return nil, fmt.Errorf("HS256 signer: %w", err)
	}

	// RS256 key pair — in production, load from a KMS or HSM
	rsaKey, err := jwt.GenerateRSAKeyPair()
	if err != nil {
		return nil, fmt.Errorf("RSA key generation: %w", err)
	}
	const keyID = "rs256-demo-key-01"

	jwks := jwt.JWKSKeySet{
		Keys: []jwt.JWK{
			jwt.PublicKeyToJWK(&rsaKey.PublicKey, keyID),
		},
	}

	return &server{
		hs256Signer:   hs256Signer,
		hs256Verifier: jwt.NewHS256Verifier(hmacSecret),
		rs256Signer:   jwt.NewRS256Signer(rsaKey, keyID),
		rs256Verifier: jwt.NewRS256Verifier(&rsaKey.PublicKey),
		jwks:          jwks,
	}, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Request / response types
// ────────────────────────────────────────────────────────────────────────────

type signRequest struct {
	Claims jwt.Claims `json:"claims"`
	Alg    string     `json:"alg"` // "HS256" or "RS256"
	TTL    int        `json:"ttl"` // seconds; default 3600
}

type signResponse struct {
	Token string `json:"token"`
}

type verifyRequest struct {
	Token string `json:"token"`
}

type verifyResponse struct {
	Valid  bool       `json:"valid"`
	Claims jwt.Claims `json:"claims,omitempty"`
	Error  string     `json:"error,omitempty"`
}

type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ────────────────────────────────────────────────────────────────────────────
// Handlers
// ────────────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *server) handleSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req signRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	if req.Claims == nil {
		req.Claims = jwt.Claims{}
	}
	ttl := time.Duration(req.TTL) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}

	// Add exp/iat if not already present
	now := time.Now().Unix()
	if _, ok := req.Claims["exp"]; !ok {
		req.Claims["exp"] = float64(now + int64(ttl.Seconds()))
	}
	if _, ok := req.Claims["iat"]; !ok {
		req.Claims["iat"] = float64(now)
	}

	var token string
	var err error

	switch req.Alg {
	case "RS256", "":
		// Default to RS256 (asymmetric is the better default for service tokens)
		token, err = s.rs256Signer.Sign(req.Claims)
	case "HS256":
		token, err = s.hs256Signer.Sign(req.Claims)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("unsupported alg %q — use 'HS256' or 'RS256'", req.Alg),
		})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, signResponse{Token: token})
}

func (s *server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'token' field"})
		return
	}

	// Try RS256 first, then HS256. In production you'd use the "kid" header
	// to look up the right key, or require the client to specify the algorithm.
	// This demo tries both so the endpoint works for both signed tokens.
	claims, err := s.rs256Verifier.Verify(req.Token)
	if err != nil {
		claims, err = s.hs256Verifier.Verify(req.Token)
	}

	if err != nil {
		writeJSON(w, http.StatusUnauthorized, verifyResponse{
			Valid: false,
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, verifyResponse{Valid: true, Claims: claims})
}

func (s *server) handleJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// JWKS must be served with the correct content type so clients can auto-discover
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600") // cache for 1 hour
	json.NewEncoder(w).Encode(s.jwks)
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// ────────────────────────────────────────────────────────────────────────────
// main
// ────────────────────────────────────────────────────────────────────────────

func main() {
	srv, err := newServer()
	if err != nil {
		log.Fatalf("server init: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/sign", srv.handleSign)
	mux.HandleFunc("/verify", srv.handleVerify)
	mux.HandleFunc("/.well-known/jwks.json", srv.handleJWKS)
	mux.HandleFunc("/health", srv.handleHealth)

	addr := ":" + port
	log.Printf("jwt-library server listening on %s", addr)
	log.Printf("  POST /sign          — sign claims (alg: HS256|RS256)")
	log.Printf("  POST /verify        — verify a token")
	log.Printf("  GET  /.well-known/jwks.json  — RS256 public keys")
	log.Printf("  GET  /health")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
