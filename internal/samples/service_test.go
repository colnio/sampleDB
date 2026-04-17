package samples_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/samples"
	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// Test DB helpers
// ---------------------------------------------------------------------------

// testDB returns a real pgxpool.Pool for integration tests, or skips the test
// if DATABASE_URL is not set.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:app@localhost:5432/sampledb"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("database not available (%v) — skipping integration test", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("database not reachable (%v) — skipping integration test", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// ---------------------------------------------------------------------------
// CheckOwner — mock DB tests
// ---------------------------------------------------------------------------

func TestCheckOwner_NonAdmin_OwnerlessIsForbidden(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	svc := samples.NewService(mock)
	userID := 5

	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	gotErr := svc.CheckOwner(context.Background(), 10, userID)
	if !errors.Is(gotErr, web.ErrForbidden) {
		t.Errorf("expected web.ErrForbidden for non-admin + ownerless sample, got %v", gotErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestCheckOwner_Admin_OwnerlessAllowed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	svc := samples.NewService(mock)
	userID := 1

	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(nil))

	mock.ExpectQuery(`SELECT COALESCE\(admin`).
		WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	gotErr := svc.CheckOwner(context.Background(), 10, userID)
	if gotErr != nil {
		t.Errorf("expected nil for admin + ownerless sample, got %v", gotErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestCheckOwner_OwnerMatch_Allowed(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	svc := samples.NewService(mock)
	userID := 7

	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}).AddRow(&userID))

	gotErr := svc.CheckOwner(context.Background(), 42, userID)
	if gotErr != nil {
		t.Errorf("expected nil for owner match, got %v", gotErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestCheckOwner_SampleNotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	svc := samples.NewService(mock)

	mock.ExpectQuery(`SELECT owner_user_id FROM samples WHERE sample_id`).
		WithArgs(999).
		WillReturnRows(pgxmock.NewRows([]string{"owner_user_id"}))

	gotErr := svc.CheckOwner(context.Background(), 999, 1)
	if !errors.Is(gotErr, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound for missing sample, got %v", gotErr)
	}
}

// ---------------------------------------------------------------------------
// GetByID — mock DB tests
// ---------------------------------------------------------------------------

func TestGetByID_WithOwnerUserID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	svc := samples.NewService(mock)
	now := time.Now()
	ownerID := 7
	username := "alice"

	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_keywords",
			"sample_owner", "owner_user_id", "username", "sample_prep", "created_at",
			"share_token", "share_visibility",
		}).AddRow(42, "Test Sample", "desc", "kw", "alice", &ownerID, &username, "", now, nil, "private"))

	mock.ExpectQuery(`SELECT attachment_id, source_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"attachment_id", "source_id", "attachment_address", "uploaded_at"}))

	smpl, err := svc.GetByID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if smpl.ID != 42 {
		t.Errorf("ID = %d, want 42", smpl.ID)
	}
	if smpl.OwnerUserID == nil || *smpl.OwnerUserID != ownerID {
		t.Errorf("OwnerUserID = %v, want %d", smpl.OwnerUserID, ownerID)
	}
	if smpl.OwnerUsername == nil || *smpl.OwnerUsername != username {
		t.Errorf("OwnerUsername = %v, want %q", smpl.OwnerUsername, username)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	svc := samples.NewService(mock)

	mock.ExpectQuery(`SELECT s\.sample_id`).
		WithArgs(999).
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_keywords",
			"sample_owner", "owner_user_id", "username", "sample_prep", "created_at",
			"share_token", "share_visibility",
		}))

	_, err = svc.GetByID(context.Background(), 999)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests — real DB
// ---------------------------------------------------------------------------

// createTestSample inserts a sample directly and returns its ID + cleanup func.
func createTestSample(t *testing.T, pool *pgxpool.Pool) (sampleID int, cleanup func()) {
	t.Helper()
	name := fmt.Sprintf("svc-test-sample-%d", time.Now().UnixNano())
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'tester') RETURNING sample_id`,
		name).Scan(&sampleID)
	if err != nil {
		t.Fatalf("createTestSample: %v", err)
	}
	cleanup = func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	}
	return sampleID, cleanup
}

func TestService_ListAll_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	_, cleanup := createTestSample(t, pool)
	defer cleanup()

	items, err := svc.ListAll(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) == 0 {
		t.Error("ListAll returned 0 items, want at least 1")
	}
}

func TestService_Search_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	name := fmt.Sprintf("searchable-sample-%d", time.Now().UnixNano())
	var id int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'tester') RETURNING sample_id`,
		name).Scan(&id)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", id)

	results, err := svc.Search(context.Background(), name[:10], "", "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var found bool
	for _, s := range results {
		if s.ID == id {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sample %d in search results, not found (got %d results)", id, len(results))
	}
}

func TestService_GetByID_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	smpl, err := svc.GetByID(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if smpl.ID != sampleID {
		t.Errorf("ID = %d, want %d", smpl.ID, sampleID)
	}
}

func TestService_CreateAndDelete_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	name := fmt.Sprintf("create-delete-test-%d", time.Now().UnixNano())
	id, err := svc.Create(context.Background(), samples.CreateParams{Name: name, Owner: "tester"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == 0 {
		t.Fatal("Create returned id=0")
	}

	if err := svc.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = svc.GetByID(context.Background(), id)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound after delete, got %v", err)
	}
}

func TestService_Update_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	err := svc.Update(context.Background(), sampleID, samples.UpdateParams{
		Name:     "Updated Name",
		Keywords: "updated,keywords",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	smpl, err := svc.GetByID(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetByID after Update: %v", err)
	}
	if smpl.Name != "Updated Name" {
		t.Errorf("Name = %q, want Updated Name", smpl.Name)
	}
}

// ---------------------------------------------------------------------------
// Attachment operations — integration
// ---------------------------------------------------------------------------

func TestService_Attachments_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	// Initially no attachments
	atts, err := svc.GetAttachments(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetAttachments: %v", err)
	}
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments for new sample, got %d", len(atts))
	}

	// Add one
	if _, err := svc.AddAttachment(context.Background(), sampleID, "uploads/abc_file.txt"); err != nil {
		t.Fatalf("AddAttachment: %v", err)
	}

	atts, err = svc.GetAttachments(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetAttachments after add: %v", err)
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].OriginalName != "file.txt" {
		t.Errorf("OriginalName = %q, want file.txt", atts[0].OriginalName)
	}

	// Delete it
	attID := atts[0].ID
	filePath, err := svc.DeleteAttachment(context.Background(), attID)
	if err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if filePath != "uploads/abc_file.txt" {
		t.Errorf("filePath = %q, want uploads/abc_file.txt", filePath)
	}

	atts, err = svc.GetAttachments(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetAttachments after delete: %v", err)
	}
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments after delete, got %d", len(atts))
	}
}

// ---------------------------------------------------------------------------
// Sharing — integration
// ---------------------------------------------------------------------------

func TestService_Sharing_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	// Generate token
	tok, err := svc.GenerateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("GenerateShareToken: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64", len(tok))
	}

	// Get by token
	smpl, err := svc.GetByShareToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("GetByShareToken: %v", err)
	}
	if smpl.ID != sampleID {
		t.Errorf("ID = %d, want %d", smpl.ID, sampleID)
	}
	if smpl.ShareVisibility != "public" {
		t.Errorf("ShareVisibility = %q, want public", smpl.ShareVisibility)
	}

	// Revoke
	if err := svc.RevokeShareToken(context.Background(), sampleID); err != nil {
		t.Fatalf("RevokeShareToken: %v", err)
	}

	// Token should no longer be found
	_, err = svc.GetByShareToken(context.Background(), tok)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound after revoke, got %v", err)
	}
}

func TestService_GenerateShareToken_TwoCallsProduceDifferentTokens(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	tok1, err := svc.GenerateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("GenerateShareToken (1): %v", err)
	}
	tok2, err := svc.GenerateShareToken(context.Background(), sampleID, "public")
	if err != nil {
		t.Fatalf("GenerateShareToken (2): %v", err)
	}
	if tok1 == tok2 {
		t.Error("expected different tokens on successive calls, got identical tokens")
	}
}

// ---------------------------------------------------------------------------
// Fabrication Steps — integration
// ---------------------------------------------------------------------------

func TestService_Steps_CRUD_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	// Empty
	steps, err := svc.GetSteps(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetSteps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for new sample, got %d", len(steps))
	}

	// Add
	desc := "spin-coat test step"
	stepDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	created, err := svc.AddStep(context.Background(), sampleID, samples.FabricationStep{
		StepOrder:   1,
		Description: desc,
		StepDate:    &stepDate,
	})
	if err != nil {
		t.Fatalf("AddStep: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("AddStep returned step with ID=0")
	}
	if created.Description != desc {
		t.Errorf("Description = %q, want %q", created.Description, desc)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE step_id = $1", created.ID)
	})

	// Get steps list
	steps, err = svc.GetSteps(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetSteps after add: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}

	// GetByID
	fetched, err := svc.GetStepByID(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetStepByID: %v", err)
	}
	if fetched.SampleID != sampleID {
		t.Errorf("SampleID = %d, want %d", fetched.SampleID, sampleID)
	}

	// Update
	if err := svc.UpdateStep(context.Background(), created.ID, samples.FabricationStep{
		StepOrder:   2,
		Description: "updated description",
	}); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}
	fetched, _ = svc.GetStepByID(context.Background(), created.ID)
	if fetched.Description != "updated description" {
		t.Errorf("after update: Description = %q, want updated description", fetched.Description)
	}

	// Delete
	if err := svc.DeleteStep(context.Background(), created.ID); err != nil {
		t.Fatalf("DeleteStep: %v", err)
	}
	steps, err = svc.GetSteps(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetSteps after delete: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps after delete, got %d", len(steps))
	}
}

func TestService_ReorderSteps_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	sampleID, cleanup := createTestSample(t, pool)
	defer cleanup()

	s1, err := svc.AddStep(context.Background(), sampleID, samples.FabricationStep{StepOrder: 1, Description: "first"})
	if err != nil {
		t.Fatalf("AddStep s1: %v", err)
	}
	s2, err := svc.AddStep(context.Background(), sampleID, samples.FabricationStep{StepOrder: 2, Description: "second"})
	if err != nil {
		t.Fatalf("AddStep s2: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE step_id IN ($1, $2)", s1.ID, s2.ID)
	})

	// Reverse order
	if err := svc.ReorderSteps(context.Background(), sampleID, []int{s2.ID, s1.ID}); err != nil {
		t.Fatalf("ReorderSteps: %v", err)
	}

	steps, err := svc.GetSteps(context.Background(), sampleID)
	if err != nil {
		t.Fatalf("GetSteps after reorder: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].ID != s2.ID {
		t.Errorf("first step after reorder = %d, want %d", steps[0].ID, s2.ID)
	}
}

// ---------------------------------------------------------------------------
// FabricationStepToAPI helper
// ---------------------------------------------------------------------------

func TestFabricationStepToAPI_WithDate(t *testing.T) {
	d := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	s := samples.FabricationStep{
		ID:          1,
		SampleID:    2,
		StepOrder:   1,
		StepDate:    &d,
		Description: "Spin coat",
		CreatedAt:   time.Now(),
	}
	item := samples.FabricationStepToAPI(s)
	if item.StepDate == nil {
		t.Fatal("StepDate is nil, want 2024-03-15")
	}
	if *item.StepDate != "2024-03-15" {
		t.Errorf("StepDate = %q, want 2024-03-15", *item.StepDate)
	}
}

func TestFabricationStepToAPI_NilDate(t *testing.T) {
	s := samples.FabricationStep{ID: 1, SampleID: 2, StepDate: nil, CreatedAt: time.Now()}
	item := samples.FabricationStepToAPI(s)
	if item.StepDate != nil {
		t.Errorf("StepDate should be nil, got %q", *item.StepDate)
	}
}

// ---------------------------------------------------------------------------
// ParsePagination helper
// ---------------------------------------------------------------------------

func TestParsePagination_Defaults(t *testing.T) {
	limit, offset := samples.ParsePagination("", "")
	if limit != 100 {
		t.Errorf("limit = %d, want 100", limit)
	}
	if offset != 0 {
		t.Errorf("offset = %d, want 0", offset)
	}
}

func TestParsePagination_Custom(t *testing.T) {
	limit, offset := samples.ParsePagination("25", "50")
	if limit != 25 {
		t.Errorf("limit = %d, want 25", limit)
	}
	if offset != 50 {
		t.Errorf("offset = %d, want 50", offset)
	}
}

func TestParsePagination_ClampsExcessLimit(t *testing.T) {
	limit, _ := samples.ParsePagination("99999", "")
	if limit != 100 {
		t.Errorf("limit = %d, want 100 (clamped)", limit)
	}
}

// ---------------------------------------------------------------------------
// Supporting queries — integration
// ---------------------------------------------------------------------------

func TestService_GetApprovedUsers_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	users, err := svc.GetApprovedUsers(context.Background())
	if err != nil {
		t.Fatalf("GetApprovedUsers: %v", err)
	}
	// Just verify it returns without error and the list is a slice.
	_ = users
}

func TestService_GetAllEquipment_Integration(t *testing.T) {
	pool := testDB(t)
	svc := samples.NewService(pool)

	equip, err := svc.GetAllEquipment(context.Background())
	if err != nil {
		t.Fatalf("GetAllEquipment: %v", err)
	}
	_ = equip
}
