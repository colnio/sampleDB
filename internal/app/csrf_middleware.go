package app

import (
	"net/http"
	"strings"

	"sampleDB/internal/auth"
)

// RequireCSRF validates the CSRF token for state-changing requests authenticated
// via session cookie. Must be applied after RequireAuth so the session is in context.
// Bearer-authenticated requests (no session CSRFToken) and unauthenticated requests
// are exempt. Checks X-CSRF-Token header first (for AJAX), then the csrf_token form field.
func RequireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodDelete {
			next(w, r)
			return
		}

		session, ok := auth.SessionFromContext(r.Context())
		if !ok || session.CSRFToken == "" {
			next(w, r)
			return
		}

		submitted := r.Header.Get("X-CSRF-Token")
		if submitted == "" {
			submitted = r.FormValue("csrf_token")
		}

		if !auth.ValidateCSRFToken(session, submitted) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"csrf token invalid"}`))
			} else {
				http.Error(w, "CSRF token invalid", http.StatusForbidden)
			}
			return
		}
		next(w, r)
	}
}
