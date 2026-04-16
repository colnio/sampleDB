package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
)

// ---------------------------------------------------------------------------
// getSampleByID — mock DB tests
// ---------------------------------------------------------------------------

func TestGetSampleByID_WithOwnerUserID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	now := time.Now()
	ownerID := 7
	username := "alice"
	ownerIDPtr := &ownerID
	usernamePtr := &username

	// getSampleByID query — includes LEFT JOIN users
	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs("42").
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_keywords",
			"sample_owner", "owner_user_id", "username", "sample_prep", "created_at",
			"share_token", "share_visibility",
		}).AddRow(42, "Test Sample", "desc", "kw", "alice", ownerIDPtr, usernamePtr, "", now, nil, "private"))

	// getAttachments query
	mock.ExpectQuery(`SELECT attachment_id, source_id`).
		WithArgs("42").
		WillReturnRows(pgxmock.NewRows([]string{"attachment_id", "source_id", "attachment_address", "uploaded_at"}))

	sample, err := getSampleByID("42")
	if err != nil {
		t.Fatalf("getSampleByID: %v", err)
	}

	if sample.OwnerUserID == nil {
		t.Fatal("OwnerUserID is nil, want 7")
	}
	if *sample.OwnerUserID != ownerID {
		t.Errorf("OwnerUserID = %d, want %d", *sample.OwnerUserID, ownerID)
	}
	if sample.OwnerUsername == nil {
		t.Fatal("OwnerUsername is nil, want alice")
	}
	if *sample.OwnerUsername != username {
		t.Errorf("OwnerUsername = %q, want %q", *sample.OwnerUsername, username)
	}
	if sample.Owner != "alice" {
		t.Errorf("Owner = %q, want alice", sample.Owner)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetSampleByID_WithoutOwnerUserID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	now := time.Now()

	// owner_user_id is NULL, username is NULL (no joined user)
	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs("10").
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_keywords",
			"sample_owner", "owner_user_id", "username", "sample_prep", "created_at",
			"share_token", "share_visibility",
		}).AddRow(10, "Legacy Sample", "", "old", "legacy-owner", nil, nil, "", now, nil, "private"))

	mock.ExpectQuery(`SELECT attachment_id, source_id`).
		WithArgs("10").
		WillReturnRows(pgxmock.NewRows([]string{"attachment_id", "source_id", "attachment_address", "uploaded_at"}))

	sample, err := getSampleByID("10")
	if err != nil {
		t.Fatalf("getSampleByID: %v", err)
	}

	if sample.OwnerUserID != nil {
		t.Errorf("OwnerUserID = %v, want nil", *sample.OwnerUserID)
	}
	if sample.OwnerUsername != nil {
		t.Errorf("OwnerUsername = %v, want nil", *sample.OwnerUsername)
	}
	if sample.Owner != "legacy-owner" {
		t.Errorf("Owner = %q, want legacy-owner", sample.Owner)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetSampleByID_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs("999").
		WillReturnError(pgx.ErrNoRows)

	_, err := getSampleByID("999")
	if err == nil {
		t.Fatal("expected error for missing sample, got nil")
	}
}

// ---------------------------------------------------------------------------
// editSampleHandler — valid owner_user_id saves owner_user_id + resolved name
// ---------------------------------------------------------------------------

// TestEditSampleHandler_WithValidOwnerUserID tests the non-HTMX path so only
// the username lookup + UPDATE are needed; renders a redirect on success.
func TestEditSampleHandler_WithValidOwnerUserID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	ownerID := 3

	// 0. requireSampleOwner check — sample has no owner (NULL), admin-only
	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	// 0a. Admin check for session user 1 → admin
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	// 1. Resolve username for owner_user_id=3
	mock.ExpectQuery(`SELECT username FROM users WHERE user_id`).
		WithArgs(ownerID).
		WillReturnRows(pgxmock.NewRows([]string{"username"}).AddRow("bob"))

	// 2. UPDATE samples — ownerName="bob", ownerUserID=3
	mock.ExpectExec(`UPDATE samples SET`).
		WithArgs("MySample", "desc", "kw", "bob", pgxmock.AnyArg(), "42").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	form := url.Values{
		"name":          {"MySample"},
		"description":   {"desc"},
		"keywords":      {"kw"},
		"owner_user_id": {fmt.Sprintf("%d", ownerID)},
	}
	req := httptest.NewRequest(http.MethodPost, "/samples/edit/42",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No HX-Request header → non-HTMX path → redirect on success
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()

	editSampleHandler(rr, req)

	// Non-HTMX success → redirect to sample detail page
	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/samples/42" {
		t.Errorf("Location = %q, want /samples/42", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// editSampleHandler — invalid (non-numeric) owner_user_id returns 400
// ---------------------------------------------------------------------------

// TestEditSampleHandler_InvalidOwnerUserID_Rejected verifies that a non-numeric
// owner_user_id causes the handler to return 400 Bad Request.
func TestEditSampleHandler_InvalidOwnerUserID_Rejected(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	// requireSampleOwner check runs before owner_user_id parsing; NULL owner = admin-only
	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	// Admin check for session user 1 → admin (lets request past ownership check)
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	form := url.Values{
		"name":          {"MySample"},
		"description":   {"desc"},
		"keywords":      {"kw"},
		"owner_user_id": {"not-a-number"},
	}
	req := httptest.NewRequest(http.MethodPost, "/samples/edit/42",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()

	editSampleHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Integration tests — real DB
// ---------------------------------------------------------------------------

func TestGetSampleByID_RealDB_OwnerUserID(t *testing.T) {
	pool := testDB(t)

	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	// Create a test user
	username := fmt.Sprintf("owner-test-user-%d", time.Now().UnixNano())
	var userID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash, is_approved) VALUES ($1, 'x', true) RETURNING user_id`,
		username).Scan(&userID)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM users WHERE user_id = $1", userID)
	})

	// Create a sample with owner_user_id set
	sampleName := fmt.Sprintf("owner-test-sample-%d", time.Now().UnixNano())
	var sampleID int
	err = pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner, owner_user_id) VALUES ($1, $2, $3) RETURNING sample_id`,
		sampleName, username, userID).Scan(&sampleID)
	if err != nil {
		t.Fatalf("insert test sample: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	})

	sampleIDStr := fmt.Sprintf("%d", sampleID)
	sample, err := getSampleByID(sampleIDStr)
	if err != nil {
		t.Fatalf("getSampleByID: %v", err)
	}

	if sample.OwnerUserID == nil {
		t.Fatal("OwnerUserID is nil, want non-nil")
	}
	if *sample.OwnerUserID != userID {
		t.Errorf("OwnerUserID = %d, want %d", *sample.OwnerUserID, userID)
	}
	if sample.OwnerUsername == nil {
		t.Fatal("OwnerUsername is nil, want non-nil")
	}
	if *sample.OwnerUsername != username {
		t.Errorf("OwnerUsername = %q, want %q", *sample.OwnerUsername, username)
	}
}

// ---------------------------------------------------------------------------
// newSampleHandler — ownership enforcement
// ---------------------------------------------------------------------------

// TestNewSampleHandler_NonAdmin_OwnerSetToSelf verifies that a non-admin creating
// a sample always has owner_user_id set to their own user ID, even when no
// owner_user_id is submitted in the form.
func TestNewSampleHandler_NonAdmin_OwnerSetToSelf(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	sessionUserID := 7

	// 1. Admin check for the session user — returns false
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(sessionUserID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	// 2. Username lookup for the session user (to populate legacy sample_owner)
	mock.ExpectQuery(`SELECT username FROM users WHERE user_id`).
		WithArgs(sessionUserID).
		WillReturnRows(pgxmock.NewRows([]string{"username"}).AddRow("alice"))

	// 3. INSERT — sample_owner must be "alice", owner_user_id set (any pointer value)
	mock.ExpectQuery(`INSERT INTO samples`).
		WithArgs("MyNewSample", "", "", "alice", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"sample_id"}).AddRow(99))

	form := url.Values{"name": {"MyNewSample"}}
	req := httptest.NewRequest(http.MethodPost, "/samples/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: sessionUserID, Username: "alice", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()

	newSampleHandler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/samples/99" {
		t.Errorf("Location = %q, want /samples/99", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// TestNewSampleHandler_NonAdmin_CannotOverrideOwner verifies that a non-admin
// submitting an explicit owner_user_id for a different user has that value
// silently ignored — the session user's ID is used instead.
func TestNewSampleHandler_NonAdmin_CannotOverrideOwner(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	sessionUserID := 7
	// submits owner_user_id=999 but it must be ignored

	// 1. Admin check returns false
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(sessionUserID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	// 2. Username lookup must use the SESSION user (7), not the form-submitted user (999)
	mock.ExpectQuery(`SELECT username FROM users WHERE user_id`).
		WithArgs(sessionUserID).
		WillReturnRows(pgxmock.NewRows([]string{"username"}).AddRow("alice"))

	// 3. INSERT with session owner, not the form-submitted owner
	mock.ExpectQuery(`INSERT INTO samples`).
		WithArgs("MySample", "", "", "alice", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"sample_id"}).AddRow(88))

	form := url.Values{
		"name":          {"MySample"},
		"owner_user_id": {"999"}, // different user — must be ignored for non-admin
	}
	req := httptest.NewRequest(http.MethodPost, "/samples/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: sessionUserID, Username: "alice", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()

	newSampleHandler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303; body: %s", rr.Code, rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// TestNewSampleHandler_Admin_CanSetArbitraryOwner verifies that an admin can
// create a sample with an owner_user_id pointing to any user.
func TestNewSampleHandler_Admin_CanSetArbitraryOwner(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	sessionUserID := 1  // admin
	targetOwnerID := 5  // different user to assign ownership to

	// 1. Admin check returns true
	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(sessionUserID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	// 2. Username lookup for the TARGET owner (not the admin)
	mock.ExpectQuery(`SELECT username FROM users WHERE user_id`).
		WithArgs(targetOwnerID).
		WillReturnRows(pgxmock.NewRows([]string{"username"}).AddRow("bob"))

	// 3. INSERT with the target owner
	mock.ExpectQuery(`INSERT INTO samples`).
		WithArgs("AdminSample", "", "", "bob", pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"sample_id"}).AddRow(77))

	form := url.Values{
		"name":          {"AdminSample"},
		"owner_user_id": {fmt.Sprintf("%d", targetOwnerID)},
	}
	req := httptest.NewRequest(http.MethodPost, "/samples/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: sessionUserID, Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}))
	rr := httptest.NewRecorder()

	newSampleHandler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303; body: %s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/samples/77" {
		t.Errorf("Location = %q, want /samples/77", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetSampleByID_RealDB_NoOwnerUserID(t *testing.T) {
	pool := testDB(t)

	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	// Create a sample with no owner_user_id (legacy text owner only)
	sampleName := fmt.Sprintf("legacy-owner-sample-%d", time.Now().UnixNano())
	var sampleID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'LegacyPerson') RETURNING sample_id`,
		sampleName).Scan(&sampleID)
	if err != nil {
		t.Fatalf("insert test sample: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	})

	sampleIDStr := fmt.Sprintf("%d", sampleID)
	sample, err := getSampleByID(sampleIDStr)
	if err != nil {
		t.Fatalf("getSampleByID: %v", err)
	}

	if sample.OwnerUserID != nil {
		t.Errorf("OwnerUserID = %d, want nil", *sample.OwnerUserID)
	}
	if sample.OwnerUsername != nil {
		t.Errorf("OwnerUsername = %q, want nil", *sample.OwnerUsername)
	}
	if sample.Owner != "LegacyPerson" {
		t.Errorf("Owner = %q, want LegacyPerson", sample.Owner)
	}
}
