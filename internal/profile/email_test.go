package profile_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/profile"
	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// Service tests — GetEmail / UpdateEmail
// ---------------------------------------------------------------------------

var (
	getEmailSQL    = regexp.QuoteMeta(`SELECT COALESCE(email, '') FROM users WHERE user_id = $1`)
	updateEmailSQL = regexp.QuoteMeta(`UPDATE users SET email = $1 WHERE user_id = $2`)
)

func TestGetEmail_ReturnsEmail(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(getEmailSQL).WithArgs(3).
		WillReturnRows(pgxmock.NewRows([]string{"email"}).AddRow("a@example.com"))

	got, err := svc.GetEmail(context.Background(), 3)
	if err != nil || got != "a@example.com" {
		t.Fatalf("GetEmail = %q, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetEmail_UserNotFound(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectQuery(getEmailSQL).WithArgs(3).WillReturnError(pgx.ErrNoRows)

	if _, err := svc.GetEmail(context.Background(), 3); !errors.Is(err, web.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateEmail_Success(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(updateEmailSQL).WithArgs("a@example.com", 3).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.UpdateEmail(context.Background(), 3, "a@example.com"); err != nil {
		t.Fatalf("UpdateEmail: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateEmail_UniqueViolation_ReturnsErrEmailTaken(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(updateEmailSQL).WithArgs("a@example.com", 3).
		WillReturnError(&pgconn.PgError{Code: "23505"})

	if err := svc.UpdateEmail(context.Background(), 3, "a@example.com"); !errors.Is(err, web.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateEmail_NoRows_ReturnsErrNotFound(t *testing.T) {
	svc, mock := newSvc(t)
	mock.ExpectExec(updateEmailSQL).WithArgs("a@example.com", 3).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	if err := svc.UpdateEmail(context.Background(), 3, "a@example.com"); !errors.Is(err, web.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests — Email
// ---------------------------------------------------------------------------

func newRenderingHandlers(t *testing.T, svc profile.Service) *profile.Handlers {
	t.Helper()
	baseDir, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatalf("abs templates dir: %v", err)
	}
	r, err := web.NewRenderer(baseDir, nil, time.UTC)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return profile.NewHandlers(svc, r, nil)
}

func TestEmail_GET_FirstTimePrompt(t *testing.T) {
	h := newRenderingHandlers(t, &mockService{})
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodGet, "/profile/email?next=%2Fbooking", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Add your email", `name="next" value="/booking"`, "Save and continue", `hx-boost="false"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if strings.Contains(body, ">Cancel<") {
		t.Error("first-time prompt must not offer Cancel")
	}
}

func TestEmail_GET_ExistingEmail(t *testing.T) {
	h := newRenderingHandlers(t, &mockService{email: "a@example.com"})
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodGet, "/profile/email", nil))

	body := w.Body.String()
	if !strings.Contains(body, `value="a@example.com"`) || !strings.Contains(body, "Update Email") {
		t.Error("edit page should prefill the current email")
	}
}

func TestEmail_POST_FirstTime_RedirectsToNext(t *testing.T) {
	svc := &mockService{}
	h := newRenderingHandlers(t, svc)
	form := url.Values{"email": {" a@example.com "}, "next": {"/booking?date=2026-09-28"}}
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodPost, "/profile/email", form))

	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/booking?date=2026-09-28" {
		t.Errorf("Location = %q", loc)
	}
	if svc.updatedEmail != "a@example.com" {
		t.Errorf("stored email = %q, want trimmed address", svc.updatedEmail)
	}
}

func TestEmail_POST_FirstTime_UnsafeNextFallsBackToDashboard(t *testing.T) {
	h := newRenderingHandlers(t, &mockService{})
	form := url.Values{"email": {"a@example.com"}, "next": {"//evil.com"}}
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodPost, "/profile/email", form))

	if loc := w.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("Location = %q, want /dashboard", loc)
	}
}

func TestEmail_POST_Update_RendersSuccess(t *testing.T) {
	svc := &mockService{email: "old@example.com"}
	h := newRenderingHandlers(t, svc)
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodPost, "/profile/email", url.Values{"email": {"new@example.com"}}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Email updated") {
		t.Error("missing success message")
	}
	if svc.updatedEmail != "new@example.com" {
		t.Errorf("stored email = %q", svc.updatedEmail)
	}
}

func TestEmail_POST_InvalidEmail_Returns400(t *testing.T) {
	svc := &mockService{}
	h := newRenderingHandlers(t, svc)
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodPost, "/profile/email", url.Values{"email": {"nope"}}))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "valid email") {
		t.Error("missing validation message")
	}
	if svc.updatedEmail != "" {
		t.Error("invalid email must not be stored")
	}
}

func TestEmail_POST_Taken_Returns409(t *testing.T) {
	h := newRenderingHandlers(t, &mockService{updateEmailErr: web.ErrEmailTaken})
	w := httptest.NewRecorder()
	h.Email(w, newAuthenticatedRequest(http.MethodPost, "/profile/email", url.Values{"email": {"a@example.com"}}))

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "already used by another account") {
		t.Error("missing duplicate message")
	}
}
