package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// GenerateCSRFToken returns a cryptographically random URL-safe token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate csrf token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ValidateCSRFToken reports whether submitted matches the token stored in session.
// Both values must be non-empty.
func ValidateCSRFToken(session Session, submitted string) bool {
	if submitted == "" || session.CSRFToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(submitted), []byte(session.CSRFToken)) == 1
}
