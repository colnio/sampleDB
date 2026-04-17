package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

func TestRedirectToHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.org/foo?bar=baz", nil)
	rr := httptest.NewRecorder()

	app.RedirectToHTTPS("example.org", "8443").ServeHTTP(rr, req)

	if rr.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected permanent redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "https://example.org:8443/foo?bar=baz" {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler := app.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}), true)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	headers := rr.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" ||
		headers.Get("X-Frame-Options") != "DENY" ||
		headers.Get("Referrer-Policy") != "no-referrer-when-downgrade" ||
		headers.Get("Strict-Transport-Security") == "" {
		t.Fatalf("security headers missing or incomplete: %+v", headers)
	}
}

// TestContextCancellationPropagated verifies that a context.Canceled error
// returned by a DB query propagates correctly through the handler stack and
// produces a non-200 response. The /s/<token> route is used because it
// requires no session authentication and issues a DB query on every request.
func TestContextCancellationPropagated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery(`SELECT`).WithArgs(pgxmock.AnyArg()).WillReturnError(context.Canceled)

	a, err := app.New(
		app.Config{DatabaseURL: "postgres://localhost/test", StaticDir: "/tmp/static"},
		mock,
		auth.NewManager(mock),
		&web.Renderer{},
		nil,
		time.Now,
		time.UTC,
	)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	a.APINotFoundHandler = http.NotFound

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/s/test-token", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code == http.StatusOK {
		t.Fatal("expected non-200 response when DB returns context.Canceled; handler may be swallowing context errors")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("DB expectation was not met: %v — handler may not be propagating the request context to DB calls", err)
	}
}
