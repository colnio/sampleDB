package app_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

const hasEmailQuery = `SELECT email IS NOT NULL FROM users WHERE user_id = \$1`

func newEmailGateApp(t *testing.T) (*app.App, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	a, err := app.New(
		app.Config{DatabaseURL: "postgres://localhost/test"},
		mock, auth.NewManager(mock), &web.Renderer{}, nil, time.Now, time.UTC,
	)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	return a, mock
}

// serveGated runs req through RequireEmail with a browser session (non-empty
// CSRF token) unless session is supplied, and reports whether the wrapped
// handler was reached.
func serveGated(a *app.App, req *http.Request, session auth.Session) (*httptest.ResponseRecorder, bool) {
	reached := false
	h := a.RequireEmail(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	req = req.WithContext(auth.ContextWithSession(req.Context(), session))
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr, reached
}

var browserSession = auth.Session{UserID: 7, Username: "alice", CSRFToken: "csrf"}

func TestRequireEmail_RedirectsWhenMissing(t *testing.T) {
	a, mock := newEmailGateApp(t)
	mock.ExpectQuery(hasEmailQuery).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"has"}).AddRow(false))

	rr, reached := serveGated(a, httptest.NewRequest(http.MethodGet, "/booking?date=2026-09-28", nil), browserSession)

	if reached {
		t.Fatal("handler reached for user without email")
	}
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rr.Code)
	}
	if loc, want := rr.Header().Get("Location"), "/profile/email?next=%2Fbooking%3Fdate%3D2026-09-28"; loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireEmail_PostRedirectDropsNext(t *testing.T) {
	a, mock := newEmailGateApp(t)
	mock.ExpectQuery(hasEmailQuery).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"has"}).AddRow(false))

	rr, reached := serveGated(a, httptest.NewRequest(http.MethodPost, "/booking", nil), browserSession)

	if reached {
		t.Fatal("handler reached for user without email")
	}
	if loc := rr.Header().Get("Location"); loc != "/profile/email" {
		t.Errorf("Location = %q, want /profile/email", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireEmail_HTMXUsesHXRedirect(t *testing.T) {
	a, mock := newEmailGateApp(t)
	mock.ExpectQuery(hasEmailQuery).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"has"}).AddRow(false))

	req := httptest.NewRequest(http.MethodGet, "/wiki", nil)
	req.Header.Set("HX-Request", "true")
	rr, reached := serveGated(a, req, browserSession)

	if reached {
		t.Fatal("handler reached for user without email")
	}
	if got := rr.Header().Get("HX-Redirect"); got != "/profile/email?next=%2Fwiki" {
		t.Errorf("HX-Redirect = %q", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireEmail_PassesWhenPresent(t *testing.T) {
	a, mock := newEmailGateApp(t)
	mock.ExpectQuery(hasEmailQuery).WithArgs(7).
		WillReturnRows(pgxmock.NewRows([]string{"has"}).AddRow(true))

	_, reached := serveGated(a, httptest.NewRequest(http.MethodGet, "/dashboard", nil), browserSession)

	if !reached {
		t.Fatal("handler not reached for user with email")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireEmail_Exemptions(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		session auth.Session
	}{
		{"bearer session", "/dashboard", auth.Session{UserID: 7, Username: "bot"}},
		{"email page", "/profile/email", browserSession},
		{"logout", "/logout", browserSession},
		{"api", "/api/notifications", browserSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, mock := newEmailGateApp(t)
			_, reached := serveGated(a, httptest.NewRequest(http.MethodGet, tt.path, nil), tt.session)
			if !reached {
				t.Fatal("exempt request did not reach handler")
			}
			// No DB query may be issued for exempt requests.
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRequireEmail_DBError(t *testing.T) {
	a, mock := newEmailGateApp(t)
	mock.ExpectQuery(hasEmailQuery).WithArgs(7).WillReturnError(errors.New("db down"))

	rr, reached := serveGated(a, httptest.NewRequest(http.MethodGet, "/dashboard", nil), browserSession)

	if reached {
		t.Fatal("handler reached despite DB error")
	}
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
