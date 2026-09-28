package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/admin"
	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// Service tests — SetEmail / GetAllUsers email column
// ---------------------------------------------------------------------------

var setEmailSQL = regexp.QuoteMeta("UPDATE users SET email = $1 WHERE user_id = $2")

func TestSetEmail_Success(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(setEmailSQL).WithArgs("a@example.com", 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.SetEmail(context.Background(), 42, "a@example.com"); err != nil {
		t.Fatalf("SetEmail: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSetEmail_Taken(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(setEmailSQL).WithArgs("a@example.com", 42).
		WillReturnError(&pgconn.PgError{Code: "23505"})

	if err := svc.SetEmail(context.Background(), 42, "a@example.com"); !errors.Is(err, web.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSetEmail_NotFound(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(setEmailSQL).WithArgs("a@example.com", 42).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	if err := svc.SetEmail(context.Background(), 42, "a@example.com"); !errors.Is(err, web.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetAllUsers_IncludesEmail(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(`COALESCE\(u\.email, ''\) AS email`).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "email", "is_approved", "admin", "group_name", "equipment_ids"}).
			AddRow(1, "alice", "alice@example.com", true, false, nil, []int32{}).
			AddRow(2, "bob", "", true, false, nil, []int32{}))

	users, err := svc.GetAllUsers(context.Background(), nil)
	if err != nil {
		t.Fatalf("GetAllUsers: %v", err)
	}
	if len(users) != 2 || users[0].Email != "alice@example.com" || users[1].Email != "" {
		t.Fatalf("users = %+v", users)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — SetEmail
// ---------------------------------------------------------------------------

func TestHandlerSetEmail_MethodNotAllowed(t *testing.T) {
	h := newHandlers(&mockService{})
	rr := httptest.NewRecorder()
	h.SetEmail(rr, newAuthenticatedRequest(http.MethodGet, "/admin/set-email", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandlerSetEmail_Success(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	form := url.Values{"user_id": {"5"}, "email": {"  bob@example.com "}}
	rr := httptest.NewRecorder()
	h.SetEmail(rr, newAuthenticatedRequest(http.MethodPost, "/admin/set-email", form))

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
	if svc.lastEmailUserID != 5 || svc.lastEmail != "bob@example.com" {
		t.Errorf("SetEmail called with (%d, %q)", svc.lastEmailUserID, svc.lastEmail)
	}
}

func TestHandlerSetEmail_InvalidEmail(t *testing.T) {
	svc := &mockService{}
	h := newHandlers(svc)
	for _, email := range []string{"", "not-an-email"} {
		form := url.Values{"user_id": {"5"}, "email": {email}}
		rr := httptest.NewRecorder()
		h.SetEmail(rr, newAuthenticatedRequest(http.MethodPost, "/admin/set-email", form))

		if loc := rr.Header().Get("Location"); rr.Code != http.StatusSeeOther || !strings.Contains(loc, "error=") {
			t.Errorf("email %q: got %d %q, want error redirect", email, rr.Code, loc)
		}
	}
	if svc.lastEmailUserID != 0 {
		t.Error("invalid email must not reach the service")
	}
}

func TestHandlerSetEmail_Taken(t *testing.T) {
	h := newHandlers(&mockService{setEmailErr: web.ErrEmailTaken})
	form := url.Values{"user_id": {"5"}, "email": {"bob@example.com"}}
	rr := httptest.NewRecorder()
	h.SetEmail(rr, newAuthenticatedRequest(http.MethodPost, "/admin/set-email", form))

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "already+in+use") {
		t.Errorf("Location = %q, want duplicate error", loc)
	}
}

func TestHandlerSetEmail_BadUserID(t *testing.T) {
	h := newHandlers(&mockService{})
	form := url.Values{"user_id": {"x"}, "email": {"bob@example.com"}}
	rr := httptest.NewRecorder()
	h.SetEmail(rr, newAuthenticatedRequest(http.MethodPost, "/admin/set-email", form))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

var _ admin.Service = (*mockService)(nil)
