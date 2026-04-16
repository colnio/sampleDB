package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"sampleDB/internal/auth"
)

// ---------------------------------------------------------------------------
// DB function tests — require a real database
// ---------------------------------------------------------------------------

func createTestSampleForSharing(t *testing.T) (sampleID int, cleanup func()) {
	t.Helper()
	pool := testDB(t)
	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	name := fmt.Sprintf("share-test-sample-%d", time.Now().UnixNano())
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'tester') RETURNING sample_id`, name).Scan(&sampleID)
	if err != nil {
		t.Fatalf("createTestSampleForSharing: %v", err)
	}
	cleanup = func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	}
	return sampleID, cleanup
}

// ---------------------------------------------------------------------------
// requireSampleOwner — unit tests (mock DB)
// ---------------------------------------------------------------------------

// TestRequireSampleOwner_NonAdmin_OwnerlessIsForbidden verifies that a non-admin
// user is denied access to a sample whose owner_user_id is NULL.
func TestRequireSampleOwner_NonAdmin_OwnerlessIsForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	userID := 5

	// owner_user_id is NULL
	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	// Admin check for user 5 → not admin
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	gotErr := requireSampleOwner(req, 10, userID)
	if !errors.Is(gotErr, errForbidden) {
		t.Errorf("expected errForbidden for non-admin + ownerless sample, got %v", gotErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// TestRequireSampleOwner_Admin_OwnerlessAllowed verifies that an admin user is
// allowed to access a sample whose owner_user_id is NULL.
func TestRequireSampleOwner_Admin_OwnerlessAllowed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	userID := 1

	// owner_user_id is NULL
	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	// Admin check for user 1 → admin
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	gotErr := requireSampleOwner(req, 10, userID)
	if gotErr != nil {
		t.Errorf("expected nil for admin + ownerless sample, got %v", gotErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// createTestOwnerAndSample creates a test user and a sample owned by that user.
// Returns sampleID, ownerUserID and a combined cleanup func.
func createTestOwnerAndSample(t *testing.T) (sampleID, ownerUserID int, cleanup func()) {
	t.Helper()
	pool := testDB(t)
	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	username := fmt.Sprintf("sample-owner-%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash, is_approved) VALUES ($1, 'x', true) RETURNING user_id`,
		username).Scan(&ownerUserID); err != nil {
		t.Fatalf("createTestOwnerAndSample: insert user: %v", err)
	}

	sampleName := fmt.Sprintf("owned-test-sample-%d", time.Now().UnixNano())
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner, owner_user_id) VALUES ($1, $2, $3) RETURNING sample_id`,
		sampleName, username, ownerUserID).Scan(&sampleID); err != nil {
		pool.Exec(context.Background(), "DELETE FROM users WHERE user_id = $1", ownerUserID)
		t.Fatalf("createTestOwnerAndSample: insert sample: %v", err)
	}

	cleanup = func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
		pool.Exec(context.Background(), "DELETE FROM users WHERE user_id = $1", ownerUserID)
	}
	return sampleID, ownerUserID, cleanup
}

// ---------------------------------------------------------------------------
// generateShareToken
// ---------------------------------------------------------------------------

func TestGenerateShareToken_ProducesUniqueTokens(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	tok1, err := generateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}
	tok2, err := generateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("generateShareToken (second): %v", err)
	}
	if tok1 == tok2 {
		t.Error("expected two different tokens, got the same")
	}
	if len(tok1) != 64 {
		t.Errorf("token length = %d, want 64", len(tok1))
	}
}

func TestGenerateShareToken_SetsVisibility(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	token, err := generateShareToken(context.Background(), sampleID, "authenticated")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}

	sample, err := getSampleByShareToken(context.Background(), token)
	if err != nil {
		t.Fatalf("getSampleByShareToken: %v", err)
	}
	if sample.ShareVisibility != "authenticated" {
		t.Errorf("visibility = %q, want \"authenticated\"", sample.ShareVisibility)
	}
}

// ---------------------------------------------------------------------------
// getSampleByShareToken
// ---------------------------------------------------------------------------

func TestGetSampleByShareToken_PublicVisibility(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	token, err := generateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}

	sample, err := getSampleByShareToken(context.Background(), token)
	if err != nil {
		t.Fatalf("getSampleByShareToken: %v", err)
	}
	if sample.ID != sampleID {
		t.Errorf("sample.ID = %d, want %d", sample.ID, sampleID)
	}
	if sample.ShareVisibility != "public" {
		t.Errorf("visibility = %q, want \"public\"", sample.ShareVisibility)
	}
}

func TestGetSampleByShareToken_NotFound(t *testing.T) {
	pool := testDB(t)
	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	_, err := getSampleByShareToken(context.Background(), "nonexistent-token-that-does-not-exist-in-db")
	if err == nil {
		t.Error("expected error for unknown token, got nil")
	}
}

// ---------------------------------------------------------------------------
// revokeShareToken
// ---------------------------------------------------------------------------

func TestRevokeShareToken(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	token, err := generateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}

	if err := revokeShareToken(context.Background(), sampleID); err != nil {
		t.Fatalf("revokeShareToken: %v", err)
	}

	// Token should no longer be found.
	_, err = getSampleByShareToken(context.Background(), token)
	if err == nil {
		t.Error("expected error after revoke, got nil")
	}
}

// ---------------------------------------------------------------------------
// GET /s/{token} handler — HTTP-level tests
// ---------------------------------------------------------------------------

func TestHandleSharedSample_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/s/sometoken", nil)
	rr := httptest.NewRecorder()
	handleSharedSample(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestHandleSharedSample_EmptyToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/s/", nil)
	rr := httptest.NewRecorder()
	handleSharedSample(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleSharedSample_PublicAccess(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	token, err := generateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}

	setTestTemplatesDir(t)
	// authManagerInstance must not be nil; use a real manager with the test pool.
	pool := testDB(t)
	origManager := authManagerInstance
	authManagerInstance = auth.NewManager(pool)
	t.Cleanup(func() { authManagerInstance = origManager })

	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	rr := httptest.NewRecorder()
	handleSharedSample(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestHandleSharedSample_AuthenticatedRequiresLogin(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	token, err := generateShareToken(context.Background(), sampleID, "authenticated")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}

	pool := testDB(t)
	origManager := authManagerInstance
	authManagerInstance = auth.NewManager(pool)
	t.Cleanup(func() { authManagerInstance = origManager })

	// No auth headers — expect redirect to /login with ?next= pointing back to the share URL.
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	rr := httptest.NewRecorder()
	handleSharedSample(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect to /login", rr.Code)
	}
	loc := rr.Header().Get("Location")
	wantPrefix := "/login?next="
	if !strings.HasPrefix(loc, wantPrefix) {
		t.Errorf("Location = %q, want prefix %q", loc, wantPrefix)
	}
}

func TestHandleSharedSample_RevokedTokenReturns404(t *testing.T) {
	sampleID, cleanup := createTestSampleForSharing(t)
	defer cleanup()

	// Generate a token then revoke it (sets share_token=NULL, visibility='private').
	token, err := generateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}
	if err := revokeShareToken(context.Background(), sampleID); err != nil {
		t.Fatalf("revokeShareToken: %v", err)
	}

	pool := testDB(t)
	origManager := authManagerInstance
	authManagerInstance = auth.NewManager(pool)
	t.Cleanup(func() { authManagerInstance = origManager })

	// The token was revoked (set to NULL) so getSampleByShareToken returns no rows.
	req := httptest.NewRequest(http.MethodGet, "/s/"+token, nil)
	rr := httptest.NewRecorder()
	handleSharedSample(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /samples/{id}/share handler
// ---------------------------------------------------------------------------

func TestHandleSampleShare_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/samples/1/share", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()
	handleSampleShare(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestHandleSampleShare_InvalidID(t *testing.T) {
	form := url.Values{"visibility": {"public"}}
	req := httptest.NewRequest(http.MethodPost, "/samples/notanumber/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withTestSession(req)
	rr := httptest.NewRecorder()
	handleSampleShare(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandleSampleShare_GeneratesAndRedirects(t *testing.T) {
	sampleID, ownerUserID, cleanup := createTestOwnerAndSample(t)
	defer cleanup()

	form := url.Values{"visibility": {"public"}}
	path := fmt.Sprintf("/samples/%d/share", sampleID)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: ownerUserID, Username: "owner", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()
	handleSampleShare(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 (body: %s)", rr.Code, rr.Body.String())
	}
	expected := fmt.Sprintf("/samples/%d", sampleID)
	if loc := rr.Header().Get("Location"); loc != expected {
		t.Errorf("Location = %q, want %q", loc, expected)
	}

	// Verify token was actually written to DB.
	sample, err := getSampleByID(fmt.Sprintf("%d", sampleID))
	if err != nil {
		t.Fatalf("getSampleByID: %v", err)
	}
	if sample.ShareToken == nil || *sample.ShareToken == "" {
		t.Error("expected share_token to be set after POST /share")
	}
}

// ---------------------------------------------------------------------------
// POST /samples/{id}/share/revoke handler
// ---------------------------------------------------------------------------

func TestHandleSampleShareRevoke_RevokesToken(t *testing.T) {
	sampleID, ownerUserID, cleanup := createTestOwnerAndSample(t)
	defer cleanup()

	// First generate a token.
	if _, err := generateShareToken(context.Background(), sampleID, "public"); err != nil {
		t.Fatalf("generateShareToken: %v", err)
	}

	path := fmt.Sprintf("/samples/%d/share/revoke", sampleID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: ownerUserID, Username: "owner", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()
	handleSampleShareRevoke(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303", rr.Code)
	}

	// Verify token was cleared.
	sample, err := getSampleByID(fmt.Sprintf("%d", sampleID))
	if err != nil {
		t.Fatalf("getSampleByID: %v", err)
	}
	if sample.ShareToken != nil {
		t.Errorf("expected share_token to be NULL after revoke, got %q", *sample.ShareToken)
	}
}
