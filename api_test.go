package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// withTestSession returns a copy of req with a test auth session in context.
func withTestSession(req *http.Request) *http.Request {
	s := auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}
	return req.WithContext(auth.ContextWithSession(req.Context(), s))
}

// ---------------------------------------------------------------------------
// writeJSON / writeError
// ---------------------------------------------------------------------------

func TestWriteJSON_SetsContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"key": "value"})
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
}

func TestWriteError_ReturnsJSONError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusNotFound, "not found")
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["error"] != "not found" {
		t.Errorf("error = %q, want %q", body["error"], "not found")
	}
}

// ---------------------------------------------------------------------------
// Auth rejection — no valid credentials → 401
// ---------------------------------------------------------------------------

func TestAPIAuth_NoAuth_Returns401(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mgr := auth.NewManager(mock)

	handler := mgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached without auth")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAPIAuth_InvalidBearer_Returns401(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mock.ExpectQuery(`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	mgr := auth.NewManager(mock)
	handler := mgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with invalid token")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
