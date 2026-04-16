package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginHandlerSuccess(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mock.Close()

	hashed, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	mock.ExpectQuery(`SELECT user_id, password_hash, is_approved, COALESCE\(deleted, false\) FROM users WHERE username = \$1`).
		WithArgs("alice").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "password_hash", "is_approved", "deleted"}).
			AddRow(7, string(hashed), true, false))

	mock.ExpectExec(`INSERT INTO sessions`).
		WithArgs(pgxmock.AnyArg(), 7, "alice", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	manager := NewManager(mock)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=alice&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	manager.LoginHandler()(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got status %d", rr.Code)
	}

	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to '/', got %q", loc)
	}

	if len(rr.Result().Cookies()) == 0 {
		t.Fatalf("expected session cookie to be set")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLoginHandlerBlocksUnapprovedUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mock.Close()

	hashed, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	mock.ExpectQuery(`SELECT user_id, password_hash, is_approved, COALESCE\(deleted, false\) FROM users WHERE username = \$1`).
		WithArgs("bob").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "password_hash", "is_approved", "deleted"}).
			AddRow(9, string(hashed), false, false))

	manager := NewManager(mock)

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=bob&password=secret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	manager.LoginHandler()(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect for unapproved user, got %d", rr.Code)
	}

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "pending+approval") {
		t.Fatalf("expected redirect to include pending approval error, got %q", loc)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRegisterHandlerCreatesUser(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM users WHERE username = \$1\)`).
		WithArgs("carol").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	mock.ExpectExec(`INSERT INTO users \(username, password_hash, is_approved\) VALUES \(\$1, \$2, false\)`).
		WithArgs("carol", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	manager := NewManager(mock)

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("username=carol&password=secretpass&confirm_password=secretpass"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	manager.RegisterHandler()(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after registration, got %d", rr.Code)
	}

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "Registration+successful") {
		t.Fatalf("expected success redirect, got %q", loc)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRegisterHandlerRejectsMismatchedPasswords(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mock.Close()

	manager := NewManager(mock)

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("username=dave&password=one&confirm_password=two"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	manager.RegisterHandler()(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect on validation error, got %d", rr.Code)
	}

	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "Passwords+do+not+match") {
		t.Fatalf("expected redirect with password mismatch error, got %q", loc)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB calls: %v", err)
	}
}

func TestContextWithSession_And_SessionFromContext(t *testing.T) {
	s := Session{UserID: 99, Username: "tester"}
	ctx := ContextWithSession(context.Background(), s)

	got, ok := SessionFromContext(ctx)
	if !ok {
		t.Fatal("expected session in context")
	}
	if got.UserID != 99 || got.Username != "tester" {
		t.Fatalf("unexpected session: %+v", got)
	}
}

func TestSessionFromContext_Missing(t *testing.T) {
	_, ok := SessionFromContext(context.Background())
	if ok {
		t.Fatal("expected no session in empty context")
	}
}

func TestMustSessionFromContext_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for missing session")
		}
	}()
	MustSessionFromContext(context.Background())
}

func TestSetCookieSecure(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	m := NewManager(mock)
	m.SetCookieSecure(true)
	if !m.cookieSecure {
		t.Fatal("expected cookieSecure to be true")
	}
	m.SetCookieSecure(false)
	if m.cookieSecure {
		t.Fatal("expected cookieSecure to be false")
	}
}

func TestSetTemplateDir(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	m := NewManager(mock)
	m.SetTemplateDir("/tmp/templates")
	if m.templateDir != "/tmp/templates" {
		t.Fatalf("unexpected templateDir: %q", m.templateDir)
	}
}

func TestTemplatePath(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()
	m := NewManager(mock)

	// Without templateDir, should prefix with "templates/".
	got := m.templatePath("login.html")
	if got != "templates/login.html" {
		t.Fatalf("unexpected path: %q", got)
	}

	// With templateDir set, should use that as base.
	m.SetTemplateDir("/app/tmpl")
	got = m.templatePath("login.html")
	if got != "/app/tmpl/login.html" {
		t.Fatalf("unexpected path: %q", got)
	}

	// Absolute path should be returned as-is.
	got = m.templatePath("/absolute/path/login.html")
	if got != "/absolute/path/login.html" {
		t.Fatalf("unexpected path for absolute: %q", got)
	}
}

func TestLogoutHandler_ClearsSessionAndRedirects(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`DELETE FROM sessions WHERE id = \$1`).
		WithArgs("test-token-123").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	m := NewManager(mock)
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "test-token-123"})
	rr := httptest.NewRecorder()

	m.LogoutHandler()(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLogoutHandler_NoCookie(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	m := NewManager(mock)
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	rr := httptest.NewRecorder()

	m.LogoutHandler()(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect even without cookie, got %d", rr.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRevokeUserSessions(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`DELETE FROM sessions WHERE user_id = \$1`).
		WithArgs(7).
		WillReturnResult(pgxmock.NewResult("DELETE", 2))

	m := NewManager(mock)
	m.RevokeUserSessions(context.Background(), 7)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestTryAuth_NoCookieNoBearer(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	m := NewManager(mock)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	_, ok, err := m.TryAuth(req)
	if ok {
		t.Fatal("expected TryAuth to return false with no credentials")
	}
	if err != nil {
		t.Fatalf("expected no error with no credentials, got: %v", err)
	}
}

func TestTryAuth_ValidCookie(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	expiresAt := time.Now().Add(24 * time.Hour)
	mock.ExpectQuery(`SELECT s\.user_id, s\.username, s\.expires_at`).
		WithArgs("valid-token").
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "expires_at"}).
			AddRow(5, "alice", expiresAt))

	m := NewManager(mock)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "valid-token"})

	session, ok, err := m.TryAuth(req)
	if !ok {
		t.Fatal("expected TryAuth to return true with valid cookie")
	}
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if session.UserID != 5 || session.Username != "alice" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSessionSurvivesManagerRestart verifies that a session created by one
// Manager instance is retrievable by a second Manager backed by the same DB,
// simulating a process restart.
func TestSessionSurvivesManagerRestart(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("failed to create mock pool: %v", err)
	}
	defer mock.Close()

	// Manager1 creates a session — must INSERT into sessions table.
	mock.ExpectExec(`INSERT INTO sessions`).
		WithArgs(pgxmock.AnyArg(), 42, "alice", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	m1 := NewManager(mock)
	session, err := m1.newSession(context.Background(), "alice", 42)
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	// Manager2 uses the same DB pool — simulates a restart with no in-memory state.
	expiresAt := time.Now().Add(24 * time.Hour)
	mock.ExpectQuery(`SELECT s\.user_id, s\.username, s\.expires_at`).
		WithArgs(session.Token).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "expires_at"}).
			AddRow(42, "alice", expiresAt))

	m2 := NewManager(mock)
	found, ok, err := m2.getSession(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected session to be found by second Manager instance")
	}
	if found.UserID != 42 || found.Username != "alice" {
		t.Fatalf("unexpected session data: %+v", found)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
