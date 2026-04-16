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

	req := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader("username=carol&password=secret&confirm_password=secret"))
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
	mock.ExpectQuery(`SELECT user_id, username, expires_at FROM sessions WHERE id = \$1`).
		WithArgs(session.Token).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "expires_at"}).
			AddRow(42, "alice", expiresAt))

	m2 := NewManager(mock)
	found, ok := m2.getSession(context.Background(), session.Token)
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
