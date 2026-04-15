package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

// setMockDB replaces the global dbPool with mock and returns a restore func.
func setMockDB(mock pgxmock.PgxPoolIface) func() {
	orig := dbPool
	dbPool = mock
	return func() { dbPool = orig }
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
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples", nil)
	limit, offset := parsePagination(req)
	if limit != 100 {
		t.Errorf("limit = %d, want 100", limit)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
}

func TestParsePagination_Custom(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples?limit=25&offset=50", nil)
	limit, offset := parsePagination(req)
	if limit != 25 {
		t.Errorf("limit = %d, want 25", limit)
	}
	if offset != 50 {
		t.Errorf("offset = %d, want 50", offset)
	}
}

func TestParsePagination_ClampsExcessLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples?limit=99999", nil)
	limit, _ := parsePagination(req)
	if limit != 100 {
		t.Errorf("limit = %d, want 100 (clamped to default for out-of-range)", limit)
	}
}

// ---------------------------------------------------------------------------
// Auth rejection — no valid credentials → redirect (not API caller 401)
// ---------------------------------------------------------------------------

func TestAPIAuth_NoAuth_Returns401(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mgr := auth.NewManager(mock)

	handler := mgr.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached without auth")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	// API paths return 401, not a browser redirect.
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken")
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/samples — success path with mock DB
// ---------------------------------------------------------------------------

func TestAPIListSamples_OK(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	now := time.Now()
	// COUNT query
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM samples`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	// Main list query (always ends with LIMIT $N OFFSET $N+1)
	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_keywords",
			"sample_owner", "owner_user_id", "username", "attachment_count", "created_at",
		}).AddRow(1, "Alpha", "desc", "kw", "alice", nil, nil, 0, now))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiListSamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body["total"] != float64(1) {
		t.Errorf("total = %v, want 1", body["total"])
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Errorf("items length = %d, want 1", len(items))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestAPIListSamples_EmptyResult(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM samples`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_keywords",
			"sample_owner", "owner_user_id", "username", "attachment_count", "created_at",
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiListSamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	items, _ := body["items"].([]any)
	if len(items) != 0 {
		t.Errorf("items not empty for no results: %v", items)
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/samples/{id} — not found
// ---------------------------------------------------------------------------

func TestAPIGetSample_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs(9999).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples/9999", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiGetSample(rr, req, "9999")

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !strings.Contains(body["error"], "not found") {
		t.Errorf("error = %q, want message containing 'not found'", body["error"])
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
// GET /api/v1/users — success path
// ---------------------------------------------------------------------------

func TestAPIListUsers_OK(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	mock.ExpectQuery(`SELECT user_id, username`).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "group"}).
			AddRow(1, "alice", "phd").
			AddRow(2, "bob", ""))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiUsersHandler(rr, req)

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
// GET /api/v1/equipment — success path
// ---------------------------------------------------------------------------

func TestAPIListEquipment_OK(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	mock.ExpectQuery(`SELECT equipment_id, name`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location"}).
			AddRow(1, "SEM", "Scanning electron microscope", "Room 102").
			AddRow(2, "XRD", "X-ray diffractometer", "Room 105"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/equipment", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiEquipmentHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	items, _ := body["items"].([]any)
	if len(items) != 2 {
		t.Errorf("items length = %d, want 2", len(items))
	}
}

// ---------------------------------------------------------------------------
// Method not allowed
// ---------------------------------------------------------------------------

func TestAPISamplesHandler_MethodNotAllowed(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/samples", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiSamplesHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// POST /api/v1/samples — bad JSON
// ---------------------------------------------------------------------------

func TestAPICreateSample_BadJSON(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/samples",
		strings.NewReader("not json"))
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiCreateSample(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestAPICreateSample_EmptyName(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	body := `{"name":"  ","description":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samples",
		strings.NewReader(body))
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiCreateSample(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// apiSamplesHandler routing
// ---------------------------------------------------------------------------

func TestAPISamplesHandler_RouteToGetByID(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	// getSample query
	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs(5).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples/5", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiSamplesHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (sample not found)", rr.Code)
	}
}

func TestAPISamplesHandler_RouteToDelete404(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	// Ownership check runs before DELETE; sample has no owner_user_id (legacy).
	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(99).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	// Attachment cleanup query — returns no attachments.
	mock.ExpectQuery(`SELECT attachment_id FROM attachments WHERE source_id`).
		WithArgs(99).
		WillReturnRows(pgxmock.NewRows([]string{"attachment_id"}))

	mock.ExpectExec(`DELETE FROM samples WHERE sample_id`).
		WithArgs(99).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/samples/99", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiSamplesHandler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// fabricationStepToAPI
// ---------------------------------------------------------------------------

func TestFabricationStepToAPI_WithDate(t *testing.T) {
	d := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	s := FabricationStep{
		ID:          1,
		SampleID:    2,
		StepOrder:   1,
		StepDate:    &d,
		Description: "Spin coat",
		CreatedAt:   time.Now(),
	}
	item := fabricationStepToAPI(s)
	if item.StepDate == nil {
		t.Fatal("StepDate is nil, want 2024-03-15")
	}
	if *item.StepDate != "2024-03-15" {
		t.Errorf("StepDate = %q, want 2024-03-15", *item.StepDate)
	}
}

func TestFabricationStepToAPI_NilDate(t *testing.T) {
	s := FabricationStep{ID: 1, SampleID: 2, StepDate: nil, CreatedAt: time.Now()}
	item := fabricationStepToAPI(s)
	if item.StepDate != nil {
		t.Errorf("StepDate should be nil, got %q", *item.StepDate)
	}
}

// ---------------------------------------------------------------------------
// Integration tests — real DB (skipped when no DB available)
// ---------------------------------------------------------------------------

func TestAPICreateAndGetSample_RoundTrip(t *testing.T) {
	pool := testDB(t)
	defer pool.Close()

	// Point global dbPool at the real test DB.
	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	// Inject a session so handlers that call MustSessionFromContext don't panic.
	sampleName := fmt.Sprintf("api-test-sample-%d", time.Now().UnixNano())

	// Create
	body, _ := json.Marshal(map[string]string{
		"name":        sampleName,
		"description": "integration test sample",
		"keywords":    "test,api",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/samples", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withTestSession(req)
	rr := httptest.NewRecorder()
	apiCreateSample(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	var created apiSampleItem
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created sample has zero ID")
	}
	if created.Name != sampleName {
		t.Errorf("name = %q, want %q", created.Name, sampleName)
	}

	// Defer cleanup
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", created.ID)
	})

	// Get
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/samples/%d", created.ID), nil)
	req2 = withTestSession(req2)
	rr2 := httptest.NewRecorder()
	apiGetSample(rr2, req2, fmt.Sprintf("%d", created.ID))

	if rr2.Code != http.StatusOK {
		t.Fatalf("get: status = %d, want 200; body: %s", rr2.Code, rr2.Body.String())
	}

	var detail apiSampleDetail
	if err := json.NewDecoder(rr2.Body).Decode(&detail); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if detail.Name != sampleName {
		t.Errorf("get name = %q, want %q", detail.Name, sampleName)
	}
	if detail.Steps == nil {
		t.Error("fabrication_steps field should be non-nil (empty array)")
	}
	if detail.Attachments == nil {
		t.Error("attachments field should be non-nil (empty array)")
	}
}

func TestAPIDeleteSample_Returns404AfterDelete(t *testing.T) {
	pool := testDB(t)
	defer pool.Close()

	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	// Create a sample directly in DB
	sampleName := fmt.Sprintf("api-delete-test-%d", time.Now().UnixNano())
	var id int
	err := pool.QueryRow(context.Background(),
		"INSERT INTO samples (sample_name) VALUES ($1) RETURNING sample_id", sampleName).Scan(&id)
	if err != nil {
		t.Fatalf("insert test sample: %v", err)
	}

	sampleIDStr := fmt.Sprintf("%d", id)

	// Delete
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/samples/"+sampleIDStr, nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()
	apiDeleteSample(rr, req, sampleIDStr)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, want 204", rr.Code)
	}

	// Get should now return 404
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/samples/"+sampleIDStr, nil)
	req2 = withTestSession(req2)
	rr2 := httptest.NewRecorder()
	apiGetSample(rr2, req2, sampleIDStr)

	if rr2.Code != http.StatusNotFound {
		t.Errorf("after delete: status = %d, want 404", rr2.Code)
	}
}

func TestAPICreateAndGetArticle_RoundTrip(t *testing.T) {
	pool := testDB(t)
	defer pool.Close()

	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	title := fmt.Sprintf("api-test-article-%d", time.Now().UnixNano())
	session := auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}

	// Ensure the user exists (use user_id=1 which should be the default admin)
	// If not, skip gracefully
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

func TestAPIListSamples_Integration(t *testing.T) {
	pool := testDB(t)
	defer pool.Close()

	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/samples", nil)
	req = withTestSession(req)
	rr := httptest.NewRecorder()

	apiListSamples(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if _, ok := body["items"]; !ok {
		t.Error("response missing 'items' field")
	}
	if _, ok := body["total"]; !ok {
		t.Error("response missing 'total' field")
	}
}
