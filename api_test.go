package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
)

// withTestSession returns a copy of req with a test auth session in context.
func withTestSession(req *http.Request) *http.Request {
	s := auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}
	return req.WithContext(auth.ContextWithSession(req.Context(), s))
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
