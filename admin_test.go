package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
)

// adminSession returns a request decorated with an admin session.
func adminSession(req *http.Request) *http.Request {
	s := auth.Session{UserID: 1, Username: "admin", ExpiresAt: time.Now().Add(time.Hour)}
	return req.WithContext(auth.ContextWithSession(req.Context(), s))
}

// TestHandleUpdateAccess_RollbackOnInsertFailure verifies that if any equipment
// permission INSERT inside handleUpdateAccess fails, the entire transaction is
// rolled back and no partial state is committed.
func TestHandleUpdateAccess_RollbackOnInsertFailure(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	payload := struct {
		UserID    int    `json:"user_id"`
		Approved  bool   `json:"approved"`
		Equipment []int  `json:"equipment"`
	}{
		UserID:   42,
		Approved: true,
		Equipment: []int{10, 20}, // two permissions; second insert will fail
	}
	body, _ := json.Marshal(payload)

	// Expect Begin
	mock.ExpectBegin()

	// Expect UPDATE users
	mock.ExpectExec(`UPDATE users SET`).
		WithArgs(true, nil, 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	// Expect DELETE existing permissions
	mock.ExpectExec(`DELETE FROM user_equipment_permissions`).
		WithArgs(42).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))

	// First INSERT succeeds
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 10).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	// Second INSERT fails — simulates a constraint violation or DB error
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 20).
		WillReturnError(errors.New("simulated DB error on second insert"))

	// Transaction must be rolled back (not committed)
	mock.ExpectRollback()

	req := httptest.NewRequest(http.MethodPost, "/admin/access", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = adminSession(req)
	rr := httptest.NewRecorder()

	handleUpdateAccess(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on insert failure, got %d; body: %s", rr.Code, rr.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations not met (transaction was NOT rolled back): %v", err)
	}
}

// TestHandleUpdateAccess_CommitsOnSuccess verifies that when all inserts succeed,
// the transaction is committed and the handler returns 200.
func TestHandleUpdateAccess_CommitsOnSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	payload := struct {
		UserID    int   `json:"user_id"`
		Approved  bool  `json:"approved"`
		Equipment []int `json:"equipment"`
	}{
		UserID:    42,
		Approved:  true,
		Equipment: []int{10, 20},
	}
	body, _ := json.Marshal(payload)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET`).
		WithArgs(true, nil, 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`DELETE FROM user_equipment_permissions`).
		WithArgs(42).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 10).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 20).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	req := httptest.NewRequest(http.MethodPost, "/admin/access", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = adminSession(req)
	rr := httptest.NewRecorder()

	handleUpdateAccess(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 on success, got %d; body: %s", rr.Code, rr.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations not met: %v", err)
	}
}
