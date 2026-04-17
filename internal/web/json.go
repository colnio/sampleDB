package web

import (
	"encoding/json"
	"log"
	"net/http"
)

// JSON encodes v as JSON and writes it to w with the given HTTP status code.
// The Content-Type header is set to "application/json".
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("web: JSON encode error: %v", err)
	}
}

// Error writes a JSON error body {"error": msg} with the given HTTP status code.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// DecodeJSON reads and decodes a JSON body from r into dst.
// It returns an error if the body is empty or malformed.
func DecodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}
