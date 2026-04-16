package web

import (
	"encoding/json"
	"net/http"
)

// JSON encodes v as JSON and writes it to w with the given HTTP status code.
// The Content-Type header is set to "application/json".
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// DecodeJSON reads and decodes a JSON body from r into dst.
// It returns an error if the body is empty or malformed.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
