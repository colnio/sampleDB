package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
)

// setTestTemplatesDir sets appConfig.TemplatesDir to the templates/ directory
// next to the test binary's working directory, and returns a restore function.
func setTestTemplatesDir(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	appConfig.TemplatesDir = filepath.Join(wd, "templates")
	t.Cleanup(func() { appConfig.TemplatesDir = "" })
}

// ---------------------------------------------------------------------------
// DB function tests — require a real database (skipped when not available)
// ---------------------------------------------------------------------------

// helper: insert a sample using the pool directly.
func createTestSample(t *testing.T) (sampleID int, cleanup func()) {
	t.Helper()
	pool := testDB(t)

	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	name := fmt.Sprintf("step-test-sample-%d", time.Now().UnixNano())
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'tester') RETURNING sample_id`, name).Scan(&sampleID)
	if err != nil {
		t.Fatalf("createTestSample: %v", err)
	}
	cleanup = func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	}
	return sampleID, cleanup
}

// ---------------------------------------------------------------------------
// getFabricationSteps — empty set for new sample
// ---------------------------------------------------------------------------

func TestGetFabricationSteps_Empty(t *testing.T) {
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	steps, err := getFabricationSteps(sampleID)
	if err != nil {
		t.Fatalf("getFabricationSteps: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps for new sample, got %d", len(steps))
	}
}

// ---------------------------------------------------------------------------
// addFabricationStep + getFabricationSteps
// ---------------------------------------------------------------------------

func TestAddAndGetFabricationSteps(t *testing.T) {
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	stepDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	desc := "Spin-coat photoresist at 3000 rpm"
	step := FabricationStep{
		StepOrder:   1,
		StepDate:    &stepDate,
		Description: desc,
	}

	created, err := addFabricationStep(sampleID, step)
	if err != nil {
		t.Fatalf("addFabricationStep: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created step has ID=0, expected non-zero")
	}
	if created.SampleID != sampleID {
		t.Errorf("SampleID = %d, want %d", created.SampleID, sampleID)
	}
	if created.Description != desc {
		t.Errorf("Description = %q, want %q", created.Description, desc)
	}

	t.Cleanup(func() {
		dbPool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE step_id = $1", created.ID)
	})

	steps, err := getFabricationSteps(sampleID)
	if err != nil {
		t.Fatalf("getFabricationSteps after add: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].ID != created.ID {
		t.Errorf("step ID mismatch: got %d, want %d", steps[0].ID, created.ID)
	}
	if steps[0].StepDate == nil {
		t.Fatal("StepDate is nil after add")
	}
	if !steps[0].StepDate.Equal(stepDate) {
		t.Errorf("StepDate = %v, want %v", steps[0].StepDate, stepDate)
	}
}

// ---------------------------------------------------------------------------
// updateFabricationStep
// ---------------------------------------------------------------------------

func TestUpdateFabricationStep(t *testing.T) {
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	step := FabricationStep{StepOrder: 1, Description: "original"}
	created, err := addFabricationStep(sampleID, step)
	if err != nil {
		t.Fatalf("addFabricationStep: %v", err)
	}
	t.Cleanup(func() {
		dbPool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE step_id = $1", created.ID)
	})

	updated := FabricationStep{StepOrder: 2, Description: "updated description"}
	if err := updateFabricationStep(created.ID, updated); err != nil {
		t.Fatalf("updateFabricationStep: %v", err)
	}

	steps, err := getFabricationSteps(sampleID)
	if err != nil {
		t.Fatalf("getFabricationSteps after update: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Description != "updated description" {
		t.Errorf("Description = %q, want %q", steps[0].Description, "updated description")
	}
	if steps[0].StepOrder != 2 {
		t.Errorf("StepOrder = %d, want 2", steps[0].StepOrder)
	}
}

// ---------------------------------------------------------------------------
// deleteFabricationStep
// ---------------------------------------------------------------------------

func TestDeleteFabricationStep(t *testing.T) {
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	step := FabricationStep{StepOrder: 1, Description: "to be deleted"}
	created, err := addFabricationStep(sampleID, step)
	if err != nil {
		t.Fatalf("addFabricationStep: %v", err)
	}

	if err := deleteFabricationStep(created.ID); err != nil {
		t.Fatalf("deleteFabricationStep: %v", err)
	}

	steps, err := getFabricationSteps(sampleID)
	if err != nil {
		t.Fatalf("getFabricationSteps after delete: %v", err)
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps after delete, got %d", len(steps))
	}
}

func TestDeleteFabricationStep_NotFound(t *testing.T) {
	testDB(t) // ensure DB is reachable, sets up cleanup

	pool := testDB(t)
	orig := dbPool
	dbPool = pool
	defer func() { dbPool = orig }()

	err := deleteFabricationStep(999999999)
	if err == nil {
		t.Fatal("expected error deleting non-existent step, got nil")
	}
}

// ---------------------------------------------------------------------------
// reorderFabricationSteps
// ---------------------------------------------------------------------------

func TestReorderFabricationSteps(t *testing.T) {
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	s1, err := addFabricationStep(sampleID, FabricationStep{StepOrder: 1, Description: "first"})
	if err != nil {
		t.Fatalf("add step 1: %v", err)
	}
	s2, err := addFabricationStep(sampleID, FabricationStep{StepOrder: 2, Description: "second"})
	if err != nil {
		t.Fatalf("add step 2: %v", err)
	}
	t.Cleanup(func() {
		dbPool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE step_id IN ($1, $2)", s1.ID, s2.ID)
	})

	// Reverse the order: put s2 first, s1 second
	if err := reorderFabricationSteps(sampleID, []int{s2.ID, s1.ID}); err != nil {
		t.Fatalf("reorderFabricationSteps: %v", err)
	}

	steps, err := getFabricationSteps(sampleID)
	if err != nil {
		t.Fatalf("getFabricationSteps after reorder: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	// s2 should now be step_order=1, s1 should be step_order=2
	if steps[0].ID != s2.ID {
		t.Errorf("first step after reorder = step %d, want %d", steps[0].ID, s2.ID)
	}
	if steps[1].ID != s1.ID {
		t.Errorf("second step after reorder = step %d, want %d", steps[1].ID, s1.ID)
	}
}

// ---------------------------------------------------------------------------
// getStepByID
// ---------------------------------------------------------------------------

func TestGetStepByID(t *testing.T) {
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	created, err := addFabricationStep(sampleID, FabricationStep{StepOrder: 1, Description: "lookup test"})
	if err != nil {
		t.Fatalf("addFabricationStep: %v", err)
	}
	t.Cleanup(func() {
		dbPool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE step_id = $1", created.ID)
	})

	fetched, err := getStepByID(created.ID)
	if err != nil {
		t.Fatalf("getStepByID: %v", err)
	}
	if fetched.ID != created.ID {
		t.Errorf("ID = %d, want %d", fetched.ID, created.ID)
	}
	if fetched.SampleID != sampleID {
		t.Errorf("SampleID = %d, want %d", fetched.SampleID, sampleID)
	}
	if fetched.Description != "lookup test" {
		t.Errorf("Description = %q, want %q", fetched.Description, "lookup test")
	}
}

// ---------------------------------------------------------------------------
// HTTP handler tests — add step returns HTML containing the new step
// ---------------------------------------------------------------------------

func TestHandleFabricationStepAdd_ReturnsHTMLWithStep(t *testing.T) {
	setTestTemplatesDir(t)
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	sampleIDStr := fmt.Sprintf("%d", sampleID)
	desc := "spin coat test step"

	form := url.Values{
		"description": {desc},
		"step_order":  {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/samples/"+sampleIDStr+"/steps",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}))

	rr := httptest.NewRecorder()
	handleFabricationStepsEndpoint(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), desc) {
		t.Errorf("response HTML does not contain step description %q", desc)
	}
	// Cleanup: delete any steps added
	t.Cleanup(func() {
		dbPool.Exec(context.Background(), "DELETE FROM fabrication_steps WHERE sample_id = $1", sampleID)
	})
}

func TestHandleFabricationStepDelete_RemovesStep(t *testing.T) {
	setTestTemplatesDir(t)
	sampleID, cleanup := createTestSample(t)
	defer cleanup()

	desc := "step to delete via handler"
	created, err := addFabricationStep(sampleID, FabricationStep{StepOrder: 1, Description: desc})
	if err != nil {
		t.Fatalf("addFabricationStep: %v", err)
	}

	stepIDStr := fmt.Sprintf("%d", created.ID)
	req := httptest.NewRequest(http.MethodPost, "/steps/"+stepIDStr+"/delete", nil)
	req.Header.Set("HX-Request", "true")
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "testuser", ExpiresAt: time.Now().Add(time.Hour)}))

	rr := httptest.NewRecorder()
	handleFabricationStepDelete(rr, req, stepIDStr)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	// Step should be gone from DB
	steps, err := getFabricationSteps(sampleID)
	if err != nil {
		t.Fatalf("getFabricationSteps after handler delete: %v", err)
	}
	for _, s := range steps {
		if s.ID == created.ID {
			t.Errorf("step %d still present after delete handler", created.ID)
		}
	}
}
