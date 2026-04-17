package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/admin"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newSvc(t *testing.T) (admin.Service, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return admin.NewService(mock), mock
}

func newHandlers(svc admin.Service) *admin.Handlers {
	return admin.NewHandlers(svc, &web.Renderer{}, nil)
}

func newAuthenticatedRequest(method, target string, body url.Values) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	session := auth.Session{UserID: 1, Username: "admin"}
	ctx := auth.ContextWithSession(req.Context(), session)
	return req.WithContext(ctx)
}

// ---------------------------------------------------------------------------
// mockService — used for handler-level tests where we don't need real DB
// ---------------------------------------------------------------------------

type mockService struct {
	equipment    []admin.Equipment
	groups       []admin.Group
	users        []admin.UserAccess
	apiUsers     []admin.APIUser
	isAdmin      bool
	updateErr    error
	setAdminErr  error
	deleteRows   int64
	deleteErr    error
	resetPwd     string
	resetErr     error
	addEqErr     error
	deleteEqErr  error
	addGrpErr    error
	deleteGrpErr error
	reportRecs   []admin.BookingRecord
	reportErr    error
	listUsersErr error
}

func (m *mockService) IsAdmin(_ context.Context, _ int) (bool, error) {
	return m.isAdmin, nil
}
func (m *mockService) GetAllEquipment(_ context.Context) ([]admin.Equipment, error) {
	return m.equipment, nil
}
func (m *mockService) GetAllGroups(_ context.Context) ([]admin.Group, error) {
	return m.groups, nil
}
func (m *mockService) GetAllUsers(_ context.Context, _ []admin.Equipment) ([]admin.UserAccess, error) {
	return m.users, nil
}
func (m *mockService) UpdateAccess(_ context.Context, _ admin.UpdateAccessParams) error {
	return m.updateErr
}
func (m *mockService) SetAdmin(_ context.Context, _ int, _ bool) error {
	return m.setAdminErr
}
func (m *mockService) DeleteUser(_ context.Context, _ int) (int64, error) {
	return m.deleteRows, m.deleteErr
}
func (m *mockService) ResetPassword(_ context.Context, _ int) (string, error) {
	return m.resetPwd, m.resetErr
}
func (m *mockService) AddEquipment(_ context.Context, _ string) error { return m.addEqErr }
func (m *mockService) DeleteEquipment(_ context.Context, _ int) error { return m.deleteEqErr }
func (m *mockService) AddGroup(_ context.Context, _ string) error     { return m.addGrpErr }
func (m *mockService) DeleteGroup(_ context.Context, _ int) error     { return m.deleteGrpErr }
func (m *mockService) GetEquipmentReport(_ context.Context, _ int, _, _ string) ([]admin.BookingRecord, error) {
	return m.reportRecs, m.reportErr
}
func (m *mockService) ListAPIUsers(_ context.Context) ([]admin.APIUser, error) {
	return m.apiUsers, m.listUsersErr
}

// ---------------------------------------------------------------------------
// Service tests — GetAllEquipment
// ---------------------------------------------------------------------------

func TestGetAllEquipment_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT equipment_id, name`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location"}).
			AddRow(1, "SEM", "Scanning electron microscope", "Lab A").
			AddRow(2, "AFM", "Atomic force microscope", "Lab B"))

	equipment, err := svc.GetAllEquipment(context.Background())
	if err != nil {
		t.Fatalf("GetAllEquipment: %v", err)
	}
	if len(equipment) != 2 {
		t.Errorf("expected 2 equipment, got %d", len(equipment))
	}
	if equipment[0].Name != "SEM" {
		t.Errorf("expected first equipment name %q, got %q", "SEM", equipment[0].Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetAllEquipment_EmptyResult(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT equipment_id, name`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location"}))

	equipment, err := svc.GetAllEquipment(context.Background())
	if err != nil {
		t.Fatalf("GetAllEquipment: %v", err)
	}
	if len(equipment) != 0 {
		t.Errorf("expected empty equipment list, got %v", equipment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — GetAllGroups
// ---------------------------------------------------------------------------

func TestGetAllGroups_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT group_id, name FROM groups`).
		WillReturnRows(pgxmock.NewRows([]string{"group_id", "name"}).
			AddRow(1, "PhD Students").
			AddRow(2, "Faculty"))

	groups, err := svc.GetAllGroups(context.Background())
	if err != nil {
		t.Fatalf("GetAllGroups: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Name != "PhD Students" {
		t.Errorf("expected first group name %q, got %q", "PhD Students", groups[0].Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetAllGroups_EmptyResult(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT group_id, name FROM groups`).
		WillReturnRows(pgxmock.NewRows([]string{"group_id", "name"}))

	groups, err := svc.GetAllGroups(context.Background())
	if err != nil {
		t.Fatalf("GetAllGroups: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(groups))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — IsAdmin
// ---------------------------------------------------------------------------

func TestIsAdmin_True(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT COALESCE\(admin, false\) FROM users`).
		WithArgs(5).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	isAdmin, err := svc.IsAdmin(context.Background(), 5)
	if err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	if !isAdmin {
		t.Error("expected admin=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestIsAdmin_False(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT COALESCE\(admin, false\) FROM users`).
		WithArgs(3).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	isAdmin, err := svc.IsAdmin(context.Background(), 3)
	if err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	if isAdmin {
		t.Error("expected admin=false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — SetAdmin
// ---------------------------------------------------------------------------

func TestSetAdmin_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET admin`).
		WithArgs(true, 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.SetAdmin(context.Background(), 42, true); err != nil {
		t.Fatalf("SetAdmin: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — DeleteUser
// ---------------------------------------------------------------------------

func TestDeleteUser_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET deleted`).
		WithArgs(10).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	rows, err := svc.DeleteUser(context.Background(), 10)
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if rows != 1 {
		t.Errorf("expected 1 row affected, got %d", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestDeleteUser_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET deleted`).
		WithArgs(999).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	rows, err := svc.DeleteUser(context.Background(), 999)
	if err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if rows != 0 {
		t.Errorf("expected 0 rows affected (not found), got %d", rows)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — AddEquipment
// ---------------------------------------------------------------------------

func TestAddEquipment_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`INSERT INTO equipment`).
		WithArgs("New SEM").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := svc.AddEquipment(context.Background(), "New SEM"); err != nil {
		t.Fatalf("AddEquipment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — DeleteEquipment (transactional)
// ---------------------------------------------------------------------------

func TestDeleteEquipment_TransactionalSuccess(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM bookings WHERE equipment_id`).
		WithArgs(5).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))
	mock.ExpectExec(`DELETE FROM user_equipment_permissions WHERE equipment_id`).
		WithArgs(5).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec(`DELETE FROM equipment WHERE equipment_id`).
		WithArgs(5).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()

	if err := svc.DeleteEquipment(context.Background(), 5); err != nil {
		t.Fatalf("DeleteEquipment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestDeleteEquipment_RollbackOnError(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM bookings WHERE equipment_id`).
		WithArgs(5).
		WillReturnError(errors.New("simulated DB error"))
	mock.ExpectRollback()

	if err := svc.DeleteEquipment(context.Background(), 5); err == nil {
		t.Fatal("expected error from DeleteEquipment, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations (transaction was NOT rolled back): %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — AddGroup
// ---------------------------------------------------------------------------

func TestAddGroup_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`INSERT INTO groups`).
		WithArgs("Research").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := svc.AddGroup(context.Background(), "Research"); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — DeleteGroup
// ---------------------------------------------------------------------------

func TestDeleteGroup_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name FROM groups WHERE group_id`).
		WithArgs(3).
		WillReturnRows(pgxmock.NewRows([]string{"name"}).AddRow("PhDs"))

	mock.ExpectExec(`UPDATE users SET .group. = NULL`).
		WithArgs("PhDs").
		WillReturnResult(pgxmock.NewResult("UPDATE", 2))

	mock.ExpectExec(`DELETE FROM groups WHERE group_id`).
		WithArgs(3).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	mock.ExpectCommit()

	if err := svc.DeleteGroup(context.Background(), 3); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestDeleteGroup_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT name FROM groups WHERE group_id`).
		WithArgs(999).
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectRollback()

	err := svc.DeleteGroup(context.Background(), 999)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — UpdateAccess (transactional)
// ---------------------------------------------------------------------------

func TestUpdateAccess_CommitsOnSuccess(t *testing.T) {
	svc, mock := newSvc(t)

	grp := "phd"
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET is_approved`).
		WithArgs(true, "phd", 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`DELETE FROM user_equipment_permissions`).
		WithArgs(42).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 10).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 20).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	if err := svc.UpdateAccess(context.Background(), admin.UpdateAccessParams{
		UserID:    42,
		Approved:  true,
		GroupName: &grp,
		Equipment: []int{10, 20},
	}); err != nil {
		t.Fatalf("UpdateAccess: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestUpdateAccess_RollbackOnInsertFailure(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE users SET is_approved`).
		WithArgs(true, nil, 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`DELETE FROM user_equipment_permissions`).
		WithArgs(42).
		WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 10).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec(`INSERT INTO user_equipment_permissions`).
		WithArgs(42, 20).
		WillReturnError(errors.New("simulated insert failure"))
	mock.ExpectRollback()

	if err := svc.UpdateAccess(context.Background(), admin.UpdateAccessParams{
		UserID:    42,
		Approved:  true,
		Equipment: []int{10, 20},
	}); err == nil {
		t.Fatal("expected error on insert failure, got nil")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations (transaction was NOT rolled back): %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — ListAPIUsers
// ---------------------------------------------------------------------------

func TestListAPIUsers_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT user_id, username`).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "group"}).
			AddRow(1, "alice", "phd").
			AddRow(2, "bob", ""))

	users, err := svc.ListAPIUsers(context.Background())
	if err != nil {
		t.Fatalf("ListAPIUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
	if users[0].Username != "alice" {
		t.Errorf("expected first user %q, got %q", "alice", users[0].Username)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

func TestUpdateAccess_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/update-access", nil)
	rr := httptest.NewRecorder()
	h.UpdateAccess(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestUpdateAccess_InvalidJSON(t *testing.T) {
	h := newHandlers(&mockService{})
	req := httptest.NewRequest(http.MethodPost, "/admin/update-access", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	session := auth.Session{UserID: 1, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h.UpdateAccess(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestUpdateAccess_ServiceError(t *testing.T) {
	h := newHandlers(&mockService{updateErr: errors.New("db error")})
	body, _ := json.Marshal(map[string]any{"user_id": 1, "approved": true})
	req := httptest.NewRequest(http.MethodPost, "/admin/update-access", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	session := auth.Session{UserID: 1, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h.UpdateAccess(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestUpdateAccess_Success(t *testing.T) {
	h := newHandlers(&mockService{})
	body, _ := json.Marshal(map[string]any{"user_id": 1, "approved": true, "equipment": []int{5}})
	req := httptest.NewRequest(http.MethodPost, "/admin/update-access", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	session := auth.Session{UserID: 1, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h.UpdateAccess(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSetAdmin_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/set-admin", nil)
	rr := httptest.NewRecorder()
	h.SetAdmin(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlerSetAdmin_Success(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"user_id": {"5"}, "is_admin": {"true"}}
	req := newAuthenticatedRequest(http.MethodPost, "/admin/set-admin", form)
	rr := httptest.NewRecorder()
	h.SetAdmin(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
}

func TestDeleteUser_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/delete-user", nil)
	rr := httptest.NewRecorder()
	h.DeleteUser(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestDeleteUser_PreventsSelfDeletion(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"user_id": {"1"}} // same as session UserID
	req := newAuthenticatedRequest(http.MethodPost, "/admin/delete-user", form)
	rr := httptest.NewRecorder()
	h.DeleteUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for self-deletion, got %d", rr.Code)
	}
}

func TestHandlerDeleteUser_NotFound(t *testing.T) {
	h := newHandlers(&mockService{deleteRows: 0})
	form := url.Values{"user_id": {"99"}}
	req := newAuthenticatedRequest(http.MethodPost, "/admin/delete-user", form)
	rr := httptest.NewRecorder()
	h.DeleteUser(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDeleteUser_JSONSuccess(t *testing.T) {
	h := newHandlers(&mockService{deleteRows: 1})
	payload := `{"user_id":5}`
	req := httptest.NewRequest(http.MethodPost, "/admin/delete-user", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	session := auth.Session{UserID: 1, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h.DeleteUser(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "true") {
		t.Errorf("expected success:true in response, got %q", rr.Body.String())
	}
}

func TestResetPassword_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/reset-password", nil)
	rr := httptest.NewRecorder()
	h.ResetPassword(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestResetPassword_UserNotFound(t *testing.T) {
	h := newHandlers(&mockService{resetErr: web.ErrNotFound})
	payload := `{"user_id":99}`
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-password", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	session := auth.Session{UserID: 1, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h.ResetPassword(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestResetPassword_Success(t *testing.T) {
	h := newHandlers(&mockService{resetPwd: "12345678"})
	payload := `{"user_id":5}`
	req := httptest.NewRequest(http.MethodPost, "/admin/reset-password", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	session := auth.Session{UserID: 1, Username: "admin"}
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h.ResetPassword(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["password"] != "12345678" {
		t.Errorf("expected password %q, got %q", "12345678", resp["password"])
	}
}

func TestAddEquipment_EmptyName(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"name": {""}}
	req := newAuthenticatedRequest(http.MethodPost, "/admin/add-equipment", form)
	rr := httptest.NewRecorder()
	h.AddEquipment(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("redirect should contain 'error', got %q", loc)
	}
}

func TestHandlerAddEquipment_Success(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"name": {"TEM"}}
	req := newAuthenticatedRequest(http.MethodPost, "/admin/add-equipment", form)
	rr := httptest.NewRecorder()
	h.AddEquipment(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
}

func TestDeleteEquipment_InvalidPath(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/delete-equipment", nil)
	rr := httptest.NewRecorder()
	h.DeleteEquipment(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDeleteEquipment_Success(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/delete-equipment/3", nil)
	rr := httptest.NewRecorder()
	h.DeleteEquipment(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAddGroup_EmptyName(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"name": {""}}
	req := newAuthenticatedRequest(http.MethodPost, "/admin/add-group", form)
	rr := httptest.NewRecorder()
	h.AddGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("redirect should contain 'error', got %q", loc)
	}
}

func TestHandlerAddGroup_Success(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"name": {"Faculty"}}
	req := newAuthenticatedRequest(http.MethodPost, "/admin/add-group", form)
	rr := httptest.NewRecorder()
	h.AddGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
}

func TestDeleteGroup_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/delete-group/3", nil)
	rr := httptest.NewRecorder()
	h.DeleteGroup(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlerDeleteGroup_NotFound(t *testing.T) {
	h := newHandlers(&mockService{deleteGrpErr: web.ErrNotFound})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/delete-group/99", nil)
	rr := httptest.NewRecorder()
	h.DeleteGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("redirect should contain 'error' for not found group, got %q", loc)
	}
}

func TestHandlerDeleteGroup_Success(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/delete-group/3", nil)
	rr := httptest.NewRecorder()
	h.DeleteGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
}

func TestEquipmentReport_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment-report", nil)
	rr := httptest.NewRecorder()
	h.EquipmentReport(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestEquipmentReport_ReturnsCSV(t *testing.T) {
	h := newHandlers(&mockService{reportRecs: []admin.BookingRecord{}})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/equipment-report?id=1&start=2024-01-01&end=2024-01-31", nil)
	rr := httptest.NewRecorder()
	h.EquipmentReport(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %q", ct)
	}
}

func TestAPIUsers_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/api/v1/users", nil)
	rr := httptest.NewRecorder()
	h.APIUsers(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAPIUsers_ReturnsJSON(t *testing.T) {
	users := []admin.APIUser{
		{ID: 1, Username: "alice", GroupName: "phd"},
		{ID: 2, Username: "bob", GroupName: ""},
	}
	h := newHandlers(&mockService{apiUsers: users})
	req := newAuthenticatedRequest(http.MethodGet, "/api/v1/users", nil)
	rr := httptest.NewRecorder()
	h.APIUsers(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["total"] != float64(2) {
		t.Errorf("expected total=2, got %v", body["total"])
	}
	if body["limit"] != float64(2) {
		t.Errorf("expected limit=2, got %v", body["limit"])
	}
	if body["offset"] != float64(0) {
		t.Errorf("expected offset=0, got %v", body["offset"])
	}
}

func TestAPIEquipment_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/api/v1/equipment", nil)
	rr := httptest.NewRecorder()
	h.APIEquipment(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestAPIEquipment_ReturnsJSON(t *testing.T) {
	equipment := []admin.Equipment{
		{ID: 1, Name: "SEM", Location: "Lab A"},
		{ID: 2, Name: "AFM", Location: "Lab B"},
	}
	h := newHandlers(&mockService{equipment: equipment})
	req := newAuthenticatedRequest(http.MethodGet, "/api/v1/equipment", nil)
	rr := httptest.NewRecorder()
	h.APIEquipment(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["total"] != float64(2) {
		t.Errorf("expected total=2, got %v", body["total"])
	}
	if body["limit"] != float64(2) {
		t.Errorf("expected limit=2, got %v", body["limit"])
	}
	if body["offset"] != float64(0) {
		t.Errorf("expected offset=0, got %v", body["offset"])
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %#v", body["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first item object, got %#v", items[0])
	}
	if first["equipment_id"] != float64(1) {
		t.Errorf("expected equipment_id=1, got %v", first["equipment_id"])
	}
	if first["name"] != "SEM" {
		t.Errorf("expected name=SEM, got %v", first["name"])
	}
	if first["location"] != "Lab A" {
		t.Errorf("expected location=Lab A, got %v", first["location"])
	}
}
