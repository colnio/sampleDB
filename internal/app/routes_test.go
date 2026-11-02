package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// stub is a no-op handler used in route tests.
var stub http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func newTestApp(t *testing.T) *app.App {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() { mock.Close() })

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
	a.APINotFoundHandler = stub
	return a
}

// TestRegisterRoutes_AllPatternsRegistered verifies that calling RegisterRoutes
// on a fresh ServeMux registers every expected URL pattern.  We use
// mux.Handler(req) which returns the matched pattern string; if a route is not
// registered the mux falls back to "/" (the catch-all) or returns "" for
// completely unknown prefixes.
func TestRegisterRoutes_AllPatternsRegistered(t *testing.T) {
	a := newTestApp(t)
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)

	tests := []struct {
		// path is the URL path to probe.
		path string
		// wantPattern is the exact pattern we expect the mux to have matched.
		wantPattern string
	}{
		// static
		{"/static/app.css", "/static/"},
		// auth
		{"/login", "/login"},
		{"/register", "/register"},
		{"/logout", "/logout"},
		// landing + samples
		{"/", "/"},
		{"/dashboard", "/dashboard"},
		{"/samples", "/samples"},
		{"/samples/new", "/samples/new"},
		{"/samples/edit/42", "/samples/edit/"},
		{"/samples/123", "/samples/"},
		{"/attachment/7", "/attachment/"},
		// booking
		{"/booking", "/booking"},
		{"/api/bookings", "/api/bookings"},
		{"/booking/delete", "/booking/delete"},
		// wiki
		{"/wiki", "/wiki"},
		{"/wiki/article", "/wiki/"},
		{"/wiki/attachment/3", "/wiki/attachment/"},
		// admin
		{"/admin", "/admin"},
		{"/admin/update-access", "/admin/update-access"},
		{"/admin/set-admin", "/admin/set-admin"},
		{"/admin/reset-password", "/admin/reset-password"},
		{"/admin/add-equipment", "/admin/add-equipment"},
		{"/admin/delete-equipment/5", "/admin/delete-equipment/"},
		{"/admin/add-group", "/admin/add-group"},
		{"/admin/delete-group/3", "/admin/delete-group/"},
		{"/admin/equipment-report", "/admin/equipment-report"},
		{"/admin/equipment/1/status", "/admin/equipment/"},
		{"/admin/equipment/1/groups/add", "/admin/equipment/"},
		{"/admin/equipment/1/groups/remove", "/admin/equipment/"},
		{"/admin/delete-user", "/admin/delete-user"},
		{"/admin/set-email", "/admin/set-email"},
		{"/admin/working-hours", "/admin/working-hours"},
		{"/admin/working-hours/exceptions", "/admin/working-hours/exceptions"},
		{"/admin/working-hours/exceptions/delete", "/admin/working-hours/exceptions/delete"},
		// steps
		{"/steps/10/edit", "/steps/"},
		// account
		{"/change-password", "/change-password"},
		{"/profile/tokens", "/profile/tokens"},
		{"/profile/email", "/profile/email"},
		{"/profile/tokens/abc", "/profile/tokens/"},
		// api
		{"/api/v1/samples", "/api/v1/samples"},
		{"/api/v1/samples/42", "/api/v1/samples/"},
		{"/api/v1/steps/10", "/api/v1/steps/"},
		{"/api/v1/attachments/7", "/api/v1/attachments/"},
		{"/api/v1/wiki", "/api/v1/wiki"},
		{"/api/v1/wiki/article", "/api/v1/wiki/"},
		{"/api/v1/users", "/api/v1/users"},
		{"/api/v1/equipment", "/api/v1/equipment"},
		{"/api/unknown", "/api/"},
		// public
		{"/s/token123", "/s/"},
		{"/agents", "/agents"},
		{"/agents/examples", "/agents/examples"},
		{"/agents/examples/foo", "/agents/examples/"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		_, gotPattern := mux.Handler(req)
		if gotPattern != tt.wantPattern {
			t.Errorf("path %q: matched pattern %q, want %q", tt.path, gotPattern, tt.wantPattern)
		}
	}
}
