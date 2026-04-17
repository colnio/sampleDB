package main

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

// TestContextCancellationPropagated verifies that a context.Canceled error
// returned by a DB query propagates correctly through the handler stack and
// produces a non-200 response rather than being silently swallowed.
//
// The /s/<token> route is used because it requires no session authentication
// and issues a DB query via r.Context() on every request. The mock is
// configured to return context.Canceled, simulating a query that was
// interrupted mid-flight (e.g. the client disconnected). A handler that
// correctly forwards r.Context() to its DB calls — rather than using
// context.Background() — will surface this error to the caller.
func TestContextCancellationPropagated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()

	// Simulate a DB query that was interrupted by context cancellation.
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
	a.AIAgentsHandler = http.NotFound
	a.AIAgentExamplesHandler = http.NotFound

	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/s/test-token", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	// The DB query returns context.Canceled; the handler must propagate the
	// error and return a non-200 response rather than masking it.
	if rr.Code == http.StatusOK {
		t.Fatal("expected non-200 response when DB returns context.Canceled; handler may be swallowing context errors")
	}

	// Verify the DB query was actually reached (expectation consumed).
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("DB expectation was not met: %v — handler may not be propagating the request context to DB calls", err)
	}
}
