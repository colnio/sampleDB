package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
)

func sessionWithCSRF(token string) auth.Session {
	return auth.Session{
		UserID:    1,
		Username:  "testuser",
		ExpiresAt: time.Now().Add(time.Hour),
		CSRFToken: token,
	}
}

func TestRequireCSRF_GetPassesThrough(t *testing.T) {
	reached := false
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	ctx := auth.ContextWithSession(req.Context(), sessionWithCSRF("tok123"))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if !reached {
		t.Fatal("GET request should pass through CSRF check")
	}
}

func TestRequireCSRF_PostWithValidFormToken(t *testing.T) {
	reached := false
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	body := strings.NewReader("csrf_token=tok123&other=value")
	req := httptest.NewRequest(http.MethodPost, "/admin/notifications", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := auth.ContextWithSession(req.Context(), sessionWithCSRF("tok123"))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if !reached {
		t.Fatal("POST with valid form csrf_token should reach handler")
	}
}

func TestRequireCSRF_PostWithValidHeader(t *testing.T) {
	reached := false
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/update-access", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", "tok123")
	ctx := auth.ContextWithSession(req.Context(), sessionWithCSRF("tok123"))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if !reached {
		t.Fatal("POST with valid X-CSRF-Token header should reach handler")
	}
}

func TestRequireCSRF_PostWithWrongToken(t *testing.T) {
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with wrong CSRF token")
	})

	body := strings.NewReader("csrf_token=wrongtoken")
	req := httptest.NewRequest(http.MethodPost, "/admin/notifications", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx := auth.ContextWithSession(req.Context(), sessionWithCSRF("correct"))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireCSRF_PostWithMissingToken(t *testing.T) {
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with missing CSRF token")
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/notifications", nil)
	ctx := auth.ContextWithSession(req.Context(), sessionWithCSRF("tok123"))
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireCSRF_BearerSessionExempt(t *testing.T) {
	reached := false
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	// Bearer token sessions have empty CSRFToken — they should be exempt.
	bearerSession := auth.Session{
		UserID:    2,
		Username:  "apiuser",
		ExpiresAt: time.Now().Add(time.Hour),
		CSRFToken: "", // no CSRF token for Bearer auth
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samples", nil)
	ctx := auth.ContextWithSession(req.Context(), bearerSession)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if !reached {
		t.Fatal("Bearer-authenticated POST should be exempt from CSRF check")
	}
}

func TestRequireCSRF_NoSessionExempt(t *testing.T) {
	reached := false
	handler := RequireCSRF(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No session in context (unauthenticated request)

	rr := httptest.NewRecorder()
	handler(rr, req)

	if !reached {
		t.Fatal("Unauthenticated POST should be exempt from CSRF check")
	}
}
