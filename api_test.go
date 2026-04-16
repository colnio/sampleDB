package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// setMockDB replaces theApp.DB with mock and returns a restore func.
func setMockDB(mock pgxmock.PgxPoolIface) func() {
	orig := theApp.DB
	theApp.DB = mock
	return func() { theApp.DB = orig }
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
// parsePagination
// ---------------------------------------------------------------------------

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki", nil)
	limit, offset := parsePagination(req)
	if limit != 100 {
		t.Errorf("limit = %d, want 100", limit)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
}

func TestParsePagination_Custom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki?limit=25&offset=50", nil)
	limit, offset := parsePagination(req)
	if limit != 25 {
		t.Errorf("limit = %d, want 25", limit)
	}
	if offset != 50 {
		t.Errorf("offset = %d, want 50", offset)
	}
}

func TestParsePagination_ClampsExcessLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki?limit=99999", nil)
	limit, _ := parsePagination(req)
	if limit != 100 {
		t.Errorf("limit = %d, want 100 (clamped to default for out-of-range)", limit)
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

// ---------------------------------------------------------------------------
// GET /api/v1/wiki — success path
// ---------------------------------------------------------------------------

func TestAPIListArticles_OK(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	now := time.Now()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM articles`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT article_id, title, created_at`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "title", "created_at", "last_modified_at"}).
			AddRow(1, "Intro", now, now).
			AddRow(2, "Advanced", now, now))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiListArticles(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["total"] != float64(2) {
		t.Errorf("total = %v, want 2", body["total"])
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/wiki/{id} — not found
// ---------------------------------------------------------------------------

func TestAPIGetArticle_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	mock.ExpectQuery(`SELECT article_id, title, content`).
		WithArgs(42).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wiki/42", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiGetArticle(rr, req, "42")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// TestAPIListUsers_OK and TestAPIListEquipment_OK have been migrated to
// internal/admin/service_test.go (Task 16).

// ---------------------------------------------------------------------------
// Integration tests — real DB (skipped when no DB available)
// ---------------------------------------------------------------------------

func TestAPICreateAndGetArticle_RoundTrip(t *testing.T) {
	pool := testDB(t)
	defer pool.Close()

	orig := theApp.DB
	theApp.DB = pool
	defer func() { theApp.DB = orig }()

	title := fmt.Sprintf("api-test-article-%d", time.Now().UnixNano())
	session := auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}

	// Ensure the user exists (use user_id=1 which should be the default admin)
	var exists bool
	if err := pool.QueryRow(context.Background(),
		"SELECT EXISTS(SELECT 1 FROM users WHERE user_id = 1)").Scan(&exists); err != nil || !exists {
		t.Skip("user_id=1 not present, skipping article round-trip test")
	}

	body, _ := json.Marshal(map[string]string{"title": title, "content": "# Test\nContent here."})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/wiki", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	apiCreateArticle(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create article: status = %d; body: %s", rr.Code, rr.Body.String())
	}

	var created apiArticleDetail
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created article has zero ID")
	}

	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM articles WHERE article_id = $1", created.ID)
	})

	// Get
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/wiki/%d", created.ID), nil)
	req2 = withTestSession(req2)
	rr2 := httptest.NewRecorder()
	apiGetArticle(rr2, req2, fmt.Sprintf("%d", created.ID))

	if rr2.Code != http.StatusOK {
		t.Fatalf("get article: status = %d; body: %s", rr2.Code, rr2.Body.String())
	}

	var detail apiArticleDetail
	json.NewDecoder(rr2.Body).Decode(&detail)
	if detail.Title != title {
		t.Errorf("title = %q, want %q", detail.Title, title)
	}
	if detail.Attachments == nil {
		t.Error("attachments should be non-nil")
	}
}
