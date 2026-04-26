package dashboard_test

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/activity"
	"sampleDB/internal/auth"
	"sampleDB/internal/dashboard"
	"sampleDB/internal/web"
)

// fakeService satisfies dashboard.Service with fixed responses.
type fakeService struct {
	kpis    dashboard.KPIs
	recents []dashboard.RecentSample
	today   []dashboard.TodayBooking
	err     error
}

func (f *fakeService) KPIs(_ context.Context) (dashboard.KPIs, error) {
	return f.kpis, f.err
}
func (f *fakeService) RecentSamples(_ context.Context, _ int) ([]dashboard.RecentSample, error) {
	return f.recents, f.err
}
func (f *fakeService) TodayBookings(_ context.Context, _ time.Time, _ *time.Location, _ int, _ bool) ([]dashboard.TodayBooking, error) {
	return f.today, f.err
}

type fakeAdmin struct{ isAdmin bool }

func (f *fakeAdmin) IsAdmin(_ context.Context, _ int) (bool, error) { return f.isAdmin, nil }

type fakeActivity struct {
	entries []activity.Entry
	err     error
}

func (f *fakeActivity) Recent(_ context.Context, _ int) ([]activity.Entry, error) {
	return f.entries, f.err
}

// buildRenderer parses the minimum set of templates needed to render /dashboard.
func buildRenderer(t *testing.T) *web.Renderer {
	t.Helper()
	r := &web.Renderer{}
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"statusChipClass":    func(s string) string { return s },
		"statusLabel":        func(s string) string { return s },
		"activityVerb":       func(s string) string { return s },
		"activityPhrase":     func(_, _, label string) string { return label },
		"activityPhraseHTML": func(_, _, label string, _ *int) template.HTML { return template.HTML(label) },
		"timeAgo":            func(t time.Time) string { return t.Format("Jan 02, 15:04") },
		"split":              func(s, _ string) []string { return strings.Split(s, ",") },
		"trim":               strings.TrimSpace,
		"formatDate":         func(t *time.Time) string { return "" },
		"add":                func(a, b int) int { return a + b },
		"mod":                func(a, b int) int { return a % b },
		"urlEscape":          func(s string) string { return s },
		"ownerSelected":      func(*int, int) bool { return false },
		"markdown":           func(s string) template.HTML { return template.HTML(s) },
		"firstInitial":       func(s string) string { return s },
		"formatDateChip":     func(s string) string { return s },
		"today":              func() string { return "" },
		"toJSON":             func(v interface{}) template.JS { return template.JS("null") },
		"csrfField":          func(token string) template.HTML { return template.HTML("") },
	}).ParseFiles("../../templates/base.html", "../../templates/header.html", "../../templates/dashboard.html")
	if err != nil {
		t.Fatalf("parse templates: %v", err)
	}
	r.Register("dashboard", tmpl)
	return r
}

func TestDashboardPage_RendersAggregatedData(t *testing.T) {
	svc := &fakeService{
		kpis: dashboard.KPIs{TotalSamples: 42, ActiveBookings: 3, WikiArticles: 8, PendingApprovals: 1},
		recents: []dashboard.RecentSample{
			{ID: 1, Name: "Widget", Status: "active", CreatedAt: time.Now()},
		},
		today: []dashboard.TodayBooking{
			{ID: 7, EquipmentName: "AFM", Username: "bob",
				StartTime: time.Now(), EndTime: time.Now().Add(time.Hour)},
		},
	}
	actLogs := &fakeActivity{entries: []activity.Entry{
		{ID: 99, ActorUsername: "alice", Verb: "created",
			TargetType: "sample", TargetLabel: "SMP-001", CreatedAt: time.Now()},
	}}
	h := dashboard.NewHandlers(svc, buildRenderer(t), &fakeAdmin{isAdmin: true}, actLogs, time.UTC)
	h.Now = func() time.Time { return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC) }

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "msmith"}))

	rr := httptest.NewRecorder()
	h.DashboardPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"msmith",
		">42<",           // total samples
		">3<",            // active bookings
		">8<",            // wiki articles
		"Requires action", // PendingApprovals > 0
		"Widget",
		"AFM",
		"alice",
		"SMP-001",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestDashboardPage_EmptyState(t *testing.T) {
	svc := &fakeService{} // all empty
	h := dashboard.NewHandlers(svc, buildRenderer(t), &fakeAdmin{}, &fakeActivity{}, time.UTC)
	h.Now = func() time.Time { return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC) }

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 2, Username: "charlie"}))

	rr := httptest.NewRecorder()
	h.DashboardPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"No samples yet",
		"No activity yet",
		"No bookings scheduled for today",
		"All clear", // PendingApprovals == 0
	} {
		if !strings.Contains(body, want) {
			t.Errorf("empty-state body missing %q", want)
		}
	}
}

func TestDashboardPage_RejectsNonGET(t *testing.T) {
	h := dashboard.NewHandlers(&fakeService{}, &web.Renderer{}, nil, nil, time.UTC)

	req := httptest.NewRequest(http.MethodPost, "/dashboard", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "u"}))

	rr := httptest.NewRecorder()
	h.DashboardPage(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rr.Code)
	}
}

func TestDashboardPage_KPIsError_Returns500(t *testing.T) {
	svc := &fakeService{err: errors.New("db down")}
	h := dashboard.NewHandlers(svc, &web.Renderer{}, nil, nil, time.UTC)

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "u"}))

	rr := httptest.NewRecorder()
	h.DashboardPage(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500", rr.Code)
	}
}

// An activity reader error should be logged but not fail the page render —
// the dashboard must still show KPIs and recents when the log is unavailable.
func TestDashboardPage_ActivityErrorDoesNotFail(t *testing.T) {
	svc := &fakeService{
		kpis:    dashboard.KPIs{},
		recents: []dashboard.RecentSample{},
		today:   []dashboard.TodayBooking{},
	}
	act := &fakeActivity{err: errors.New("activity backend offline")}
	h := dashboard.NewHandlers(svc, buildRenderer(t), &fakeAdmin{}, act, time.UTC)
	h.Now = func() time.Time { return time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC) }

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(),
		auth.Session{UserID: 1, Username: "u"}))

	rr := httptest.NewRecorder()
	h.DashboardPage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("activity error must not fail the request: status=%d body=%s", rr.Code, rr.Body.String())
	}
}
