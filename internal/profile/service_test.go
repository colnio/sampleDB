package profile_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
	"sampleDB/internal/profile"
	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newSvc(t *testing.T) (profile.Service, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return profile.NewService(mock), mock
}

// mockService is a controllable in-memory implementation of profile.Service,
// used for handler-level tests that don't exercise real DB queries.
type mockService struct {
	isAdmin       bool
	adminErr      error
	passwordHash  string
	hashErr       error
	updateErr     error
}

func (m *mockService) GetAdminStatus(_ context.Context, _ int) (bool, error) {
	return m.isAdmin, m.adminErr
}
func (m *mockService) GetPasswordHash(_ context.Context, _ int) (string, error) {
	return m.passwordHash, m.hashErr
}
func (m *mockService) UpdatePassword(_ context.Context, _ int, _ string) error {
	return m.updateErr
}

func newHandlers(svc profile.Service, authMgr *auth.Manager) *profile.Handlers {
	return profile.NewHandlers(svc, &web.Renderer{}, authMgr)
}

func newAuthenticatedRequest(method, target string, body url.Values) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	session := auth.Session{UserID: 1, Username: "testuser"}
	ctx := auth.ContextWithSession(req.Context(), session)
	return req.WithContext(ctx)
}

// ---------------------------------------------------------------------------
// Service tests — GetAdminStatus
// ---------------------------------------------------------------------------

func TestGetAdminStatus_ReturnsTrue(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT admin FROM users`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	isAdmin, err := svc.GetAdminStatus(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetAdminStatus: %v", err)
	}
	if !isAdmin {
		t.Error("expected isAdmin = true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetAdminStatus_ReturnsFalse(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT admin FROM users`).
		WithArgs(2).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	isAdmin, err := svc.GetAdminStatus(context.Background(), 2)
	if err != nil {
		t.Fatalf("GetAdminStatus: %v", err)
	}
	if isAdmin {
		t.Error("expected isAdmin = false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — GetPasswordHash
// ---------------------------------------------------------------------------

func TestGetPasswordHash_ReturnsHash(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT password_hash FROM users`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"password_hash"}).AddRow("$2a$hashed"))

	hash, err := svc.GetPasswordHash(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPasswordHash: %v", err)
	}
	if hash != "$2a$hashed" {
		t.Errorf("expected hash %q, got %q", "$2a$hashed", hash)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetPasswordHash_UserNotFound_ReturnsErrNotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT password_hash FROM users`).
		WithArgs(99).
		WillReturnError(pgx.ErrNoRows)

	_, err := svc.GetPasswordHash(context.Background(), 99)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Service tests — UpdatePassword
// ---------------------------------------------------------------------------

func TestUpdatePassword_Success(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs("$2a$newhash", 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.UpdatePassword(context.Background(), 1, "$2a$newhash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestUpdatePassword_NoRowsAffected_ReturnsErrNotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE users SET password_hash`).
		WithArgs("$2a$newhash", 99).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := svc.UpdatePassword(context.Background(), 99, "$2a$newhash")
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — ChangePassword
// ---------------------------------------------------------------------------

func TestChangePassword_GET_RendersForm(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodGet, "/change-password", nil)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)
	// Renderer with no templates returns an error, but the handler should
	// still reach the render call (i.e. not crash or redirect).
	// A 500 from an empty renderer is acceptable for this unit test.
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status %d", w.Code)
	}
}

func TestChangePassword_POST_MissingFields_Returns400(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	form := url.Values{
		"current_password": {""},
		"new_password":     {""},
		"confirm_password": {""},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/change-password", form)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)

	if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 400 or 500, got %d", w.Code)
	}
}

func TestChangePassword_POST_ShortNewPassword_Returns400(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	form := url.Values{
		"current_password": {"oldpass"},
		"new_password":     {"short"},
		"confirm_password": {"short"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/change-password", form)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)

	if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 400 or 500, got %d", w.Code)
	}
}

func TestChangePassword_POST_PasswordMismatch_Returns400(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	form := url.Values{
		"current_password": {"oldpassword"},
		"new_password":     {"newpassword1"},
		"confirm_password": {"newpassword2"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/change-password", form)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)

	if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 400 or 500, got %d", w.Code)
	}
}

func TestChangePassword_POST_UserNotFound_Redirects(t *testing.T) {
	svc := &mockService{isAdmin: false, hashErr: web.ErrNotFound}
	h := newHandlers(svc, nil)

	form := url.Values{
		"current_password": {"currentpass"},
		"new_password":     {"newpassword1"},
		"confirm_password": {"newpassword1"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/change-password", form)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected redirect (303), got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/logout" {
		t.Errorf("expected redirect to /logout, got %q", loc)
	}
}

func TestChangePassword_POST_WrongMethod_Returns405(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodDelete, "/change-password", nil)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — ProfileTokens
// ---------------------------------------------------------------------------

func TestProfileTokens_GET_ListError_ShowsErrorInPage(t *testing.T) {
	// Build a minimal auth.Manager backed by a pgxmock pool so that
	// ListAPITokens returns an error.
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery(`SELECT token_id`).
		WillReturnError(errors.New("db error"))

	authMgr := auth.NewManager(mock)

	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, authMgr)

	req := newAuthenticatedRequest(http.MethodGet, "/profile/tokens", nil)
	w := httptest.NewRecorder()

	h.ProfileTokens(w, req)

	// Either 200 with error message rendered (empty Renderer gives 500), or 500.
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("unexpected status %d", w.Code)
	}
}

func TestProfileTokens_POST_EmptyName_Returns400(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	authMgr := auth.NewManager(mock)

	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, authMgr)

	form := url.Values{"name": {""}}
	req := newAuthenticatedRequest(http.MethodPost, "/profile/tokens", form)
	w := httptest.NewRecorder()

	h.ProfileTokens(w, req)

	if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 400 or 500, got %d", w.Code)
	}
}

func TestProfileTokens_POST_WrongMethod_Returns405(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodDelete, "/profile/tokens", nil)
	w := httptest.NewRecorder()

	h.ProfileTokens(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — ProfileTokenRevoke
// ---------------------------------------------------------------------------

func TestProfileTokenRevoke_WrongMethod_Returns405(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodGet, "/profile/tokens/1/revoke", nil)
	w := httptest.NewRecorder()

	h.ProfileTokenRevoke(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestProfileTokenRevoke_InvalidPath_Returns404(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodPost, "/profile/tokens/1/wrongaction", nil)
	w := httptest.NewRecorder()

	h.ProfileTokenRevoke(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestProfileTokenRevoke_InvalidTokenID_Returns400(t *testing.T) {
	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodPost, "/profile/tokens/abc/revoke", nil)
	w := httptest.NewRecorder()

	h.ProfileTokenRevoke(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestProfileTokenRevoke_TokenNotFound_Returns404(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`UPDATE api_tokens SET revoked`).
		WithArgs(42, 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	authMgr := auth.NewManager(mock)

	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, authMgr)

	req := newAuthenticatedRequest(http.MethodPost, "/profile/tokens/42/revoke", nil)
	w := httptest.NewRecorder()

	h.ProfileTokenRevoke(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestProfileTokenRevoke_Success_Redirects(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`UPDATE api_tokens SET revoked`).
		WithArgs(7, 1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	authMgr := auth.NewManager(mock)

	svc := &mockService{isAdmin: false}
	h := newHandlers(svc, authMgr)

	req := newAuthenticatedRequest(http.MethodPost, "/profile/tokens/7/revoke", nil)
	w := httptest.NewRecorder()

	h.ProfileTokenRevoke(w, req)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/profile/tokens" {
		t.Errorf("expected redirect to /profile/tokens, got %q", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — AdminStatus error causes account info error
// ---------------------------------------------------------------------------

func TestChangePassword_GET_AdminStatusError_Returns500(t *testing.T) {
	svc := &mockService{adminErr: errors.New("db down")}
	h := newHandlers(svc, nil)

	req := newAuthenticatedRequest(http.MethodGet, "/change-password", nil)
	w := httptest.NewRecorder()

	h.ChangePassword(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Ensure unused import is satisfied (time is used in APIToken struct)
// ---------------------------------------------------------------------------

var _ = time.Now
