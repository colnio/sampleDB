package auth

import (
	"context"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"
)

// ---------------------------------------------------------------------------
// GenerateToken / HashToken
// ---------------------------------------------------------------------------

func TestGenerateToken_Length(t *testing.T) {
	plain, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken error: %v", err)
	}
	// plain is 32 random bytes → 64 hex chars
	if len(plain) != 64 {
		t.Errorf("plain token length = %d, want 64", len(plain))
	}
	// hash is SHA-256 → 64 hex chars
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
	// plain must be valid hex
	if _, err := hex.DecodeString(plain); err != nil {
		t.Errorf("plain token is not valid hex: %v", err)
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	p1, _, _ := GenerateToken()
	p2, _, _ := GenerateToken()
	if p1 == p2 {
		t.Error("two GenerateToken calls returned identical tokens")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("sometoken")
	h2 := HashToken("sometoken")
	if h1 != h2 {
		t.Error("HashToken is not deterministic")
	}
}

func TestHashToken_Distinguishes(t *testing.T) {
	if HashToken("abc") == HashToken("xyz") {
		t.Error("different inputs produced the same hash")
	}
}

func TestHashToken_Length(t *testing.T) {
	h := HashToken("test")
	if len(h) != 64 {
		t.Errorf("HashToken length = %d, want 64", len(h))
	}
}

// ---------------------------------------------------------------------------
// CreateAPIToken
// ---------------------------------------------------------------------------

func TestCreateAPIToken_Success(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`INSERT INTO api_tokens \(user_id, token_hash, name\) VALUES \(\$1, \$2, \$3\)`).
		WithArgs(42, pgxmock.AnyArg(), "my key").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	m := NewManager(mock)
	plain, err := m.CreateAPIToken(context.Background(), 42, "my key")
	if err != nil {
		t.Fatalf("CreateAPIToken error: %v", err)
	}
	if len(plain) != 64 {
		t.Errorf("plain token length = %d, want 64", len(plain))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCreateAPIToken_DBError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`INSERT INTO api_tokens`).
		WillReturnError(pgx.ErrTxClosed)

	m := NewManager(mock)
	_, err = m.CreateAPIToken(context.Background(), 1, "fail")
	if err == nil {
		t.Error("expected error from DB, got nil")
	}
}

// ---------------------------------------------------------------------------
// ValidateAPIToken
// ---------------------------------------------------------------------------

func TestValidateAPIToken_Success(t *testing.T) {
	plain, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mock.ExpectQuery(`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash = \$1`).
		WithArgs(hash).
		WillReturnRows(pgxmock.NewRows([]string{"token_id", "user_id", "revoked"}).
			AddRow(1, 7, false))
	mock.ExpectExec(`UPDATE api_tokens SET last_used_at = NOW\(\) WHERE token_id = \$1`).
		WithArgs(1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	m := NewManager(mock)
	userID, err := m.ValidateAPIToken(context.Background(), plain)
	if err != nil {
		t.Fatalf("ValidateAPIToken error: %v", err)
	}
	if userID != 7 {
		t.Errorf("userID = %d, want 7", userID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestValidateAPIToken_Revoked(t *testing.T) {
	plain, hash, _ := GenerateToken()

	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mock.ExpectQuery(`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash = \$1`).
		WithArgs(hash).
		WillReturnRows(pgxmock.NewRows([]string{"token_id", "user_id", "revoked"}).
			AddRow(2, 7, true))

	m := NewManager(mock)
	_, err := m.ValidateAPIToken(context.Background(), plain)
	if err == nil {
		t.Error("expected error for revoked token, got nil")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Errorf("error should mention 'revoked', got: %v", err)
	}
}

func TestValidateAPIToken_WrongToken(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	fakeHash := HashToken("wrongtoken")
	mock.ExpectQuery(`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash = \$1`).
		WithArgs(fakeHash).
		WillReturnError(pgx.ErrNoRows)

	m := NewManager(mock)
	_, err := m.ValidateAPIToken(context.Background(), "wrongtoken")
	if err == nil {
		t.Error("expected error for unknown token, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListAPITokens
// ---------------------------------------------------------------------------

func TestListAPITokens_ReturnsRows(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`SELECT token_id, user_id, .* FROM api_tokens WHERE user_id = \$1`).
		WithArgs(5).
		WillReturnRows(pgxmock.NewRows([]string{"token_id", "user_id", "name", "created_at", "last_used_at", "revoked"}).
			AddRow(10, 5, "ios", now, nil, false).
			AddRow(11, 5, "mcp", now, &now, false))

	m := NewManager(mock)
	tokens, err := m.ListAPITokens(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListAPITokens error: %v", err)
	}
	if len(tokens) != 2 {
		t.Errorf("got %d tokens, want 2", len(tokens))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// RevokeAPIToken
// ---------------------------------------------------------------------------

func TestRevokeAPIToken_Success(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mock.ExpectExec(`UPDATE api_tokens SET revoked = true WHERE token_id = \$1 AND user_id = \$2`).
		WithArgs(10, 5).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	m := NewManager(mock)
	if err := m.RevokeAPIToken(context.Background(), 10, 5); err != nil {
		t.Fatalf("RevokeAPIToken error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRevokeAPIToken_NotFound(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mock.ExpectExec(`UPDATE api_tokens SET revoked = true WHERE token_id = \$1 AND user_id = \$2`).
		WithArgs(99, 5).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	m := NewManager(mock)
	err := m.RevokeAPIToken(context.Background(), 99, 5)
	if err == nil {
		t.Error("expected error for missing token, got nil")
	}
}

// ---------------------------------------------------------------------------
// RequireAuth middleware — cookie path and bearer path
// ---------------------------------------------------------------------------

func TestRequireAuth_CookiePath(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	// newSession inserts into DB.
	mock.ExpectExec(`INSERT INTO sessions`).
		WithArgs(pgxmock.AnyArg(), 1, "alice", pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	m := NewManager(mock)
	// Inject a valid session via DB-backed newSession.
	session, err := m.newSession(context.Background(), "alice", 1)
	if err != nil {
		t.Fatalf("newSession: %v", err)
	}

	// RequireAuth will SELECT the session from DB (joined with users).
	mock.ExpectQuery(`SELECT s\.user_id, s\.username, s\.expires_at`).
		WithArgs(session.Token).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "username", "expires_at"}).
			AddRow(1, "alice", time.Now().Add(24*time.Hour)))

	reached := false
	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	req := newRequest("GET", "/", "")
	req.AddCookie(&http.Cookie{Name: "session_token", Value: session.Token})
	rr := newRecorder()
	handler(rr, req)

	if !reached {
		t.Error("handler was not reached via cookie auth")
	}
}

func TestRequireAuth_BearerPath(t *testing.T) {
	plain, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	// ValidateAPIToken queries
	mock.ExpectQuery(`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash = \$1`).
		WithArgs(hash).
		WillReturnRows(pgxmock.NewRows([]string{"token_id", "user_id", "revoked"}).
			AddRow(1, 9, false))
	mock.ExpectExec(`UPDATE api_tokens SET last_used_at`).
		WithArgs(1).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	// username lookup
	mock.ExpectQuery(`SELECT username FROM users WHERE user_id = \$1`).
		WithArgs(9).
		WillReturnRows(pgxmock.NewRows([]string{"username"}).AddRow("bob"))

	m := NewManager(mock)

	reached := false
	var gotUserID int
	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		s, _ := SessionFromContext(r.Context())
		gotUserID = s.UserID
	})

	req := newRequest("GET", "/api/v1/samples", "")
	req.Header.Set("Authorization", "Bearer "+plain)
	rr := newRecorder()
	handler(rr, req)

	if !reached {
		t.Error("handler was not reached via bearer auth")
	}
	if gotUserID != 9 {
		t.Errorf("context userID = %d, want 9", gotUserID)
	}
}

func TestRequireAuth_InvalidToken_Returns401(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	mock.ExpectQuery(`SELECT token_id, user_id, revoked FROM api_tokens WHERE token_hash = \$1`).
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(pgx.ErrNoRows)

	m := NewManager(mock)
	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached with invalid token")
	})

	req := newRequest("GET", "/api/v1/samples", "")
	req.Header.Set("Authorization", "Bearer invalidtoken")
	rr := newRecorder()
	handler(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireAuth_NoCookie_Redirects(t *testing.T) {
	mock, _ := pgxmock.NewPool()
	defer mock.Close()

	m := NewManager(mock)
	handler := m.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be reached without auth")
	})

	req := newRequest("GET", "/", "")
	rr := newRecorder()
	handler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 redirect", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// helpers reused across tests
// ---------------------------------------------------------------------------

func newRequest(method, path, body string) *http.Request {
	var b *strings.Reader
	if body != "" {
		b = strings.NewReader(body)
	} else {
		b = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, b)
	return req
}

func newRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}
