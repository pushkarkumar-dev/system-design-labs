// Package main implements an HTTP server that exposes the password hashing service.
//
// Endpoints:
//
//	POST /hash   {password} → {hash, version}
//	POST /verify {password, hash, version} → {ok, needsRehash}
//	GET  /health → {status, algorithm, pepperVersion}
//
// Configuration via environment variables:
//
//	PEPPER_V1=<hex>   current pepper secret (default: demo value — CHANGE IN PRODUCTION)
//	PEPPER_V2=<hex>   previous pepper secret for rotation (optional)
//	PORT=<port>       listen port (default: 8080)
//
// Security note: this server does NOT implement rate limiting. In production,
// the /verify endpoint must be protected by exponential backoff, account lockout,
// or an upstream rate limiter. See "What the toy misses" in the lab MDX.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"dev.pushkar/password-hashing/pkg/passwords"
)

// hashRequest is the request body for POST /hash.
type hashRequest struct {
	Password string `json:"password"`
}

// hashResponse is the response body for POST /hash.
type hashResponse struct {
	Hash    string `json:"hash"`
	Version int    `json:"version"`
}

// verifyRequest is the request body for POST /verify.
type verifyRequest struct {
	Password      string `json:"password"`
	Hash          string `json:"hash"`
	PepperVersion int    `json:"pepperVersion"`
}

// verifyResponse is the response body for POST /verify.
type verifyResponse struct {
	OK          bool   `json:"ok"`
	NeedsRehash bool   `json:"needsRehash"`
	Message     string `json:"message,omitempty"`
}

// healthResponse is the response body for GET /health.
type healthResponse struct {
	Status        string `json:"status"`
	Algorithm     string `json:"algorithm"`
	PepperVersion int    `json:"pepperVersion"`
	Memory        string `json:"argon2Memory"`
}

func main() {
	store := buildKeyStore()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /hash", handleHash(store))
	mux.HandleFunc("POST /verify", handleVerify(store))
	mux.HandleFunc("GET /health", handleHealth(store))

	port := getEnv("PORT", "8080")
	log.Printf("password-hashing server listening on :%s", port)
	log.Printf("algorithm: Argon2id (m=64MB, t=3, p=4)")
	log.Printf("pepper version: %d", store.CurrentKey.Version)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// buildKeyStore constructs the PepperKeyStore from environment variables.
//
// PEPPER_V1 is the current pepper. PEPPER_V2 (optional) is a previous pepper
// for rotation — if set, it becomes the previous key and V1 becomes the current.
func buildKeyStore() *passwords.PepperKeyStore {
	// Default pepper is a demo value — never use this in production.
	// Generate with: head -c 32 /dev/urandom | hexdump -v -e '/1 "%02x"'
	pepperV1 := getEnv("PEPPER_V1", "6465666175746c745f70657070657231") // "defaultpepper1" in hex

	params := passwords.DefaultArgon2Params()
	store := passwords.NewPepperKeyStore(pepperV1, params)

	// If PEPPER_V2 is set, rotate: V2 becomes current, V1 becomes previous.
	if pepperV2 := os.Getenv("PEPPER_V2"); pepperV2 != "" {
		store.AddCurrentKey(2, pepperV2)
	}

	return store
}

func handleHash(store *passwords.PepperKeyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req hashRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Password == "" {
			http.Error(w, "password is required", http.StatusBadRequest)
			return
		}

		result, err := store.Hash(req.Password)
		if err != nil {
			log.Printf("hash error: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		writeJSON(w, http.StatusOK, hashResponse{
			Hash:    result.Hash,
			Version: result.PepperVersion,
		})
	}
}

func handleVerify(store *passwords.PepperKeyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req verifyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if req.Password == "" || req.Hash == "" {
			http.Error(w, "password and hash are required", http.StatusBadRequest)
			return
		}

		vr := store.Verify(req.Password, req.Hash, req.PepperVersion)

		resp := verifyResponse{
			OK:          vr.OK,
			NeedsRehash: vr.NeedsRehash,
		}
		if vr.NeedsRehash && vr.OK {
			resp.Message = "password verified — caller should re-hash with current params"
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func handleHealth(store *passwords.PepperKeyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:        "ok",
			Algorithm:     "argon2id",
			PepperVersion: store.CurrentKey.Version,
			Memory:        fmt.Sprintf("%d KiB (%d MB)", store.Params.Memory, store.Params.Memory/1024),
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
