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
	"time"

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
	equipment        []admin.Equipment
	groups           []admin.Group
	users            []admin.UserAccess
	apiUsers         []admin.APIUser
	isAdmin          bool
	updateErr        error
	setAdminErr      error
	deleteRows       int64
	deleteErr        error
	resetPwd         string
	resetErr         error
	addEqErr         error
	deleteEqErr      error
	setEqStatusErr   error
	addEqGroupErr    error
	removeEqGroupErr error
	addGrpErr        error
	deleteGrpErr     error
	reportRecs       []admin.BookingRecord
	reportErr        error
	listUsersErr     error

	// notification state
	notifs        []admin.Notification
	listNotifsErr error
	createNotif   admin.Notification
	createNotifErr error
	deleteNotifErr error

	// captured args (handler tests assert on these)
	lastEquipmentID    int
	lastEquipmentStat  string
	lastEquipmentGroup int
	lastNotifMsg       string
	lastNotifLevel     string
	lastDeleteNotifID  int64
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
func (m *mockService) SetEquipmentStatus(_ context.Context, id int, status string) error {
	m.lastEquipmentID = id
	m.lastEquipmentStat = status
	return m.setEqStatusErr
}
func (m *mockService) AddEquipmentGroup(_ context.Context, equipmentID, groupID int) error {
	m.lastEquipmentID = equipmentID
	m.lastEquipmentGroup = groupID
	return m.addEqGroupErr
}
func (m *mockService) RemoveEquipmentGroup(_ context.Context, equipmentID, groupID int) error {
	m.lastEquipmentID = equipmentID
	m.lastEquipmentGroup = groupID
	return m.removeEqGroupErr
}
func (m *mockService) AddGroup(_ context.Context, _ string) error { return m.addGrpErr }
func (m *mockService) DeleteGroup(_ context.Context, _ int) error { return m.deleteGrpErr }
func (m *mockService) GetEquipmentReport(_ context.Context, _ int, _, _ string) ([]admin.BookingRecord, error) {
	return m.reportRecs, m.reportErr
}
func (m *mockService) ListAPIUsers(_ context.Context) ([]admin.APIUser, error) {
	return m.apiUsers, m.listUsersErr
}
func (m *mockService) ListNotifications(_ context.Context) ([]admin.Notification, error) {
	return m.notifs, m.listNotifsErr
}
func (m *mockService) CreateNotification(_ context.Context, msg, level, _ string) (admin.Notification, error) {
	m.lastNotifMsg = msg
	m.lastNotifLevel = level
	return m.createNotif, m.createNotifErr
}
func (m *mockService) DeleteNotification(_ context.Context, id int64) error {
	m.lastDeleteNotifID = id
	return m.deleteNotifErr
}

// ---------------------------------------------------------------------------
// Service tests — GetAllEquipment
// ---------------------------------------------------------------------------

func TestGetAllEquipment_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT e\.equipment_id`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location", "status", "groups"}).
			AddRow(1, "SEM", "Scanning electron microscope", "Lab A", "online", []string{"Barbaros"}).
			AddRow(2, "AFM", "Atomic force microscope", "Lab B", "maintenance", []string{}))

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
	if equipment[0].Status != "online" {
		t.Errorf("expected first equipment status %q, got %q", "online", equipment[0].Status)
	}
	if len(equipment[0].Groups) != 1 || equipment[0].Groups[0] != "Barbaros" {
		t.Errorf("expected first equipment groups [Barbaros], got %v", equipment[0].Groups)
	}
	if equipment[1].Status != "maintenance" {
		t.Errorf("expected second equipment status %q, got %q", "maintenance", equipment[1].Status)
	}
	if len(equipment[1].Groups) != 0 {
		t.Errorf("expected empty groups for AFM, got %v", equipment[1].Groups)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetAllEquipment_EmptyResult(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT e\.equipment_id`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location", "status", "groups"}))

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
// Service tests — ResetPassword
// ---------------------------------------------------------------------------

func TestResetPassword_ServiceReturnsMinEntropyPassword(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs(pgxmock.AnyArg(), 7).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	pw, err := svc.ResetPassword(context.Background(), 7)
	if err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if len(pw) < 12 {
		t.Errorf("expected password length >= 12, got %d (%q)", len(pw), pw)
	}
	if pw == "" {
		t.Error("expected non-empty password")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestResetPassword_ServiceNotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs(pgxmock.AnyArg(), 999).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	_, err := svc.ResetPassword(context.Background(), 999)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
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
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["success"] != true {
		t.Errorf("expected success:true in response, got %v", resp)
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
	h := newHandlers(&mockService{resetPwd: "123456789012"})
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
	if resp["password"] != "123456789012" {
		t.Errorf("expected password %q, got %q", "123456789012", resp["password"])
	}
	if resp["note"] == "" {
		t.Error("expected non-empty note field in response")
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
	if _, ok := body["limit"]; ok {
		t.Errorf("unexpected limit field in response")
	}
	if _, ok := body["offset"]; ok {
		t.Errorf("unexpected offset field in response")
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
	if _, ok := body["limit"]; ok {
		t.Errorf("unexpected limit field in response")
	}
	if _, ok := body["offset"]; ok {
		t.Errorf("unexpected offset field in response")
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %#v", body["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first item object, got %#v", items[0])
	}
	if first["id"] != float64(1) {
		t.Errorf("expected id=1, got %v", first["id"])
	}
	if first["name"] != "SEM" {
		t.Errorf("expected name=SEM, got %v", first["name"])
	}
	if first["location"] != "Lab A" {
		t.Errorf("expected location=Lab A, got %v", first["location"])
	}
}

// ---------------------------------------------------------------------------
// Service tests — SetEquipmentStatus
// ---------------------------------------------------------------------------

func TestSetEquipmentStatus_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE equipment SET status`).
		WithArgs("maintenance", 7).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.SetEquipmentStatus(context.Background(), 7, "maintenance"); err != nil {
		t.Fatalf("SetEquipmentStatus: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestSetEquipmentStatus_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE equipment SET status`).
		WithArgs("offline", 999).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := svc.SetEquipmentStatus(context.Background(), 999, "offline")
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestSetEquipmentStatus_InvalidStatus(t *testing.T) {
	svc, _ := newSvc(t)

	if err := svc.SetEquipmentStatus(context.Background(), 1, "bogus"); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

// ---------------------------------------------------------------------------
// Service tests — AddEquipmentGroup / RemoveEquipmentGroup
// ---------------------------------------------------------------------------

func TestAddEquipmentGroup_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`INSERT INTO equipment_group_access`).
		WithArgs(5, 9).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := svc.AddEquipmentGroup(context.Background(), 5, 9); err != nil {
		t.Fatalf("AddEquipmentGroup: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestRemoveEquipmentGroup_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`DELETE FROM equipment_group_access`).
		WithArgs(5, 9).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	if err := svc.RemoveEquipmentGroup(context.Background(), 5, 9); err != nil {
		t.Fatalf("RemoveEquipmentGroup: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — SetEquipmentStatus / AddEquipmentGroup / RemoveEquipmentGroup
// ---------------------------------------------------------------------------

func TestHandlerSetEquipmentStatus_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/equipment/5/status", nil)
	rr := httptest.NewRecorder()
	h.SetEquipmentStatus(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlerSetEquipmentStatus_InvalidPath(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/abc/status", url.Values{"status": {"online"}})
	rr := httptest.NewRecorder()
	h.SetEquipmentStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-numeric id, got %d", rr.Code)
	}
}

func TestHandlerSetEquipmentStatus_EmptyStatus(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/5/status", url.Values{"status": {""}})
	rr := httptest.NewRecorder()
	h.SetEquipmentStatus(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("expected error redirect, got %q", loc)
	}
}

func TestHandlerSetEquipmentStatus_Success(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/5/status", url.Values{"status": {"maintenance"}})
	rr := httptest.NewRecorder()
	h.SetEquipmentStatus(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	if svc.lastEquipmentID != 5 || svc.lastEquipmentStat != "maintenance" {
		t.Errorf("service called with unexpected args: id=%d, status=%q", svc.lastEquipmentID, svc.lastEquipmentStat)
	}
}

func TestHandlerSetEquipmentStatus_NotFound(t *testing.T) {
	h := newHandlers(&mockService{setEqStatusErr: web.ErrNotFound})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/99/status", url.Values{"status": {"online"}})
	rr := httptest.NewRecorder()
	h.SetEquipmentStatus(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("expected error redirect, got %q", loc)
	}
}

func TestHandlerAddEquipmentGroup_Success(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/5/groups/add", url.Values{"group_id": {"3"}})
	rr := httptest.NewRecorder()
	h.AddEquipmentGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	if svc.lastEquipmentID != 5 || svc.lastEquipmentGroup != 3 {
		t.Errorf("service called with unexpected args: eq=%d, group=%d", svc.lastEquipmentID, svc.lastEquipmentGroup)
	}
}

func TestHandlerAddEquipmentGroup_MissingGroupID(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/5/groups/add", url.Values{"group_id": {""}})
	rr := httptest.NewRecorder()
	h.AddEquipmentGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("expected error redirect, got %q", loc)
	}
}

func TestHandlerRemoveEquipmentGroup_Success(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/5/groups/remove", url.Values{"group_id": {"3"}})
	rr := httptest.NewRecorder()
	h.RemoveEquipmentGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if svc.lastEquipmentID != 5 || svc.lastEquipmentGroup != 3 {
		t.Errorf("service called with unexpected args: eq=%d, group=%d", svc.lastEquipmentID, svc.lastEquipmentGroup)
	}
}

func TestHandlerRemoveEquipmentGroup_ServiceError(t *testing.T) {
	h := newHandlers(&mockService{removeEqGroupErr: errors.New("db error")})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/equipment/5/groups/remove", url.Values{"group_id": {"3"}})
	rr := httptest.NewRecorder()
	h.RemoveEquipmentGroup(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("expected error redirect, got %q", loc)
	}
}

// ---------------------------------------------------------------------------
// Service tests — Notifications
// ---------------------------------------------------------------------------

func TestListNotifications_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)
	now := time.Now()

	mock.ExpectQuery(`SELECT id, message, level, author, created_at FROM notifications`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "message", "level", "author", "created_at"}).
			AddRow(int64(1), "Maintenance tonight", "warning", "admin", now).
			AddRow(int64(2), "System back online", "info", "ops", now))

	notifs, err := svc.ListNotifications(context.Background())
	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if len(notifs) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifs))
	}
	if notifs[0].Message != "Maintenance tonight" {
		t.Errorf("unexpected message: %q", notifs[0].Message)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListNotifications_DBError(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(`SELECT id, message, level, author, created_at FROM notifications`).
		WillReturnError(errors.New("db error"))

	_, err := svc.ListNotifications(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err2 := mock.ExpectationsWereMet(); err2 != nil {
		t.Error(err2)
	}
}

func TestCreateNotification_Success(t *testing.T) {
	svc, mock := newSvc(t)
	now := time.Now()

	mock.ExpectQuery(`INSERT INTO notifications`).
		WithArgs("System alert", "critical", "admin").
		WillReturnRows(pgxmock.NewRows([]string{"id", "message", "level", "author", "created_at"}).
			AddRow(int64(5), "System alert", "critical", "admin", now))

	n, err := svc.CreateNotification(context.Background(), "System alert", "critical", "admin")
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}
	if n.ID != 5 || n.Level != "critical" {
		t.Errorf("unexpected notification: %+v", n)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestDeleteNotification_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`DELETE FROM notifications WHERE id = \$1`).
		WithArgs(int64(3)).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	if err := svc.DeleteNotification(context.Background(), 3); err != nil {
		t.Fatalf("DeleteNotification: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — Notifications
// ---------------------------------------------------------------------------

func TestHandlerListNotifications_Success(t *testing.T) {
	svc := &mockService{
		notifs: []admin.Notification{
			{ID: 1, Message: "hello", Level: "info", Author: "admin"},
		},
	}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodGet, "/api/notifications", nil)
	rr := httptest.NewRecorder()
	h.ListNotifications(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var out []admin.Notification
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].Message != "hello" {
		t.Errorf("unexpected response: %+v", out)
	}
}

func TestHandlerListNotifications_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/api/notifications", nil)
	rr := httptest.NewRecorder()
	h.ListNotifications(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlerListNotifications_ServiceError(t *testing.T) {
	h := newHandlers(&mockService{listNotifsErr: errors.New("db down")})
	req := newAuthenticatedRequest(http.MethodGet, "/api/notifications", nil)
	rr := httptest.NewRecorder()
	h.ListNotifications(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestHandlerCreateNotification_Success(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodPost, "/admin/notifications", url.Values{
		"message": {"Scheduled maintenance"},
		"level":   {"warning"},
	})
	rr := httptest.NewRecorder()
	h.CreateNotification(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	if svc.lastNotifMsg != "Scheduled maintenance" || svc.lastNotifLevel != "warning" {
		t.Errorf("service called with msg=%q level=%q", svc.lastNotifMsg, svc.lastNotifLevel)
	}
}

func TestHandlerCreateNotification_MissingMessage(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/notifications", url.Values{
		"message": {""},
		"level":   {"info"},
	})
	rr := httptest.NewRecorder()
	h.CreateNotification(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("expected error redirect, got %q", loc)
	}
}

func TestHandlerCreateNotification_InvalidLevelDefaultsToInfo(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodPost, "/admin/notifications", url.Values{
		"message": {"test"},
		"level":   {"supersecret"},
	})
	rr := httptest.NewRecorder()
	h.CreateNotification(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if svc.lastNotifLevel != "info" {
		t.Errorf("expected level to default to 'info', got %q", svc.lastNotifLevel)
	}
}

func TestHandlerCreateNotification_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/notifications", nil)
	rr := httptest.NewRecorder()
	h.CreateNotification(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlerDeleteNotification_Success(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	req := newAuthenticatedRequest(http.MethodPost, "/admin/notifications/7/delete", nil)
	rr := httptest.NewRecorder()
	h.DeleteNotification(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("expected success redirect, got %q", loc)
	}
	if svc.lastDeleteNotifID != 7 {
		t.Errorf("expected notif ID 7, got %d", svc.lastDeleteNotifID)
	}
}

func TestHandlerDeleteNotification_InvalidPath(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/notifications/abc/delete", nil)
	rr := httptest.NewRecorder()
	h.DeleteNotification(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandlerDeleteNotification_ServiceError(t *testing.T) {
	h := newHandlers(&mockService{deleteNotifErr: errors.New("db error")})
	req := newAuthenticatedRequest(http.MethodPost, "/admin/notifications/3/delete", nil)
	rr := httptest.NewRecorder()
	h.DeleteNotification(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("expected error redirect, got %q", loc)
	}
}

func TestHandlerDeleteNotification_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	req := newAuthenticatedRequest(http.MethodGet, "/admin/notifications/3/delete", nil)
	rr := httptest.NewRecorder()
	h.DeleteNotification(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}
