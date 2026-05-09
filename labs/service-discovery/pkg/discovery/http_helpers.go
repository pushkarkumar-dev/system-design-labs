// http_helpers.go — shared HTTP utilities for RegistryServer and HealthServer.
package discovery

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
)

// decodeJSON decodes JSON from r into v.
func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

// writeJSON encodes v as JSON and writes it to w with the correct Content-Type.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("json encode error: %v", err)
	}
}
