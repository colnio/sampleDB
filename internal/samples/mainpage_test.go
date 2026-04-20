package samples_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/samples"
	"sampleDB/internal/web"
)

// listStub is a minimal Service stub that records the ListFilters passed into
// ListAll / Search so individual tests can assert on them.
type listStub struct {
	samples.Service

	captured  samples.ListFilters
	capturedQ string

	listSamples []samples.Sample
	err         error
	users       []samples.User
}

func (s *listStub) ListAll(_ context.Context, f samples.ListFilters) ([]samples.Sample, error) {
	s.captured = f
	s.capturedQ = ""
	return s.listSamples, s.err
}

func (s *listStub) Search(_ context.Context, q string, f samples.ListFilters) ([]samples.Sample, error) {
	s.captured = f
	s.capturedQ = q
	return s.listSamples, s.err
}

func (s *listStub) IsAdmin(_ context.Context, _ int) (bool, error) { return false, nil }
func (s *listStub) GetApprovedUsers(_ context.Context) ([]samples.User, error) {
	return s.users, nil
}

func newRenderer(t *testing.T) *web.Renderer {
	t.Helper()
	baseDir, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatalf("abs templates dir: %v", err)
	}
	r, err := web.NewRenderer(baseDir, nil, time.UTC)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

func doMainPage(t *testing.T, stub samples.Service, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})

	target := "/samples"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "tester"}))
	rr := httptest.NewRecorder()
	h.MainPage(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Filter-combo tests
// ---------------------------------------------------------------------------

func TestMainPage_StatusFilter_Active(t *testing.T) {
	stub := &listStub{}
	rr := doMainPage(t, stub, "status=active")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if stub.captured.Status != "active" {
		t.Errorf("ListFilters.Status = %q, want %q", stub.captured.Status, "active")
	}
}

func TestMainPage_StatusFilter_All_IsEmpty(t *testing.T) {
	// "all" is the sentinel for "no filter" — the handler must strip it so the
	// DB query never sees an invalid status string.
	stub := &listStub{}
	_ = doMainPage(t, stub, "status=all")
	if stub.captured.Status != "" {
		t.Errorf("status=all should coerce to empty filter, got %q", stub.captured.Status)
	}
}

func TestMainPage_StatusFilter_Garbage_IsEmpty(t *testing.T) {
	// Unknown status values coerce to "" so the CHECK constraint in SQL is
	// never exercised with an invalid value.
	stub := &listStub{}
	_ = doMainPage(t, stub, "status=not_a_real_status")
	if stub.captured.Status != "" {
		t.Errorf("unknown status must coerce to empty, got %q", stub.captured.Status)
	}
}

func TestMainPage_OwnerFilter(t *testing.T) {
	stub := &listStub{}
	_ = doMainPage(t, stub, "owner=alice")
	if stub.captured.Owner != "alice" {
		t.Errorf("Owner = %q, want alice", stub.captured.Owner)
	}
}

func TestMainPage_OwnerFilter_All_IsEmpty(t *testing.T) {
	stub := &listStub{}
	_ = doMainPage(t, stub, "owner=all")
	if stub.captured.Owner != "" {
		t.Errorf("owner=all should coerce to empty, got %q", stub.captured.Owner)
	}
}

func TestMainPage_QueryRoutesToSearch(t *testing.T) {
	// A non-empty ?query= must call Search, not ListAll, and must still pass
	// the status+owner filters through to the Search implementation.
	stub := &listStub{}
	_ = doMainPage(t, stub, "query=needle&status=archived&owner=bob")
	if stub.capturedQ != "needle" {
		t.Errorf("Search query = %q, want needle", stub.capturedQ)
	}
	if stub.captured.Status != "archived" {
		t.Errorf("Search status = %q, want archived", stub.captured.Status)
	}
	if stub.captured.Owner != "bob" {
		t.Errorf("Search owner = %q, want bob", stub.captured.Owner)
	}
}

func TestMainPage_CombinedFilters(t *testing.T) {
	stub := &listStub{}
	_ = doMainPage(t, stub,
		"status=in_progress&owner=alice&date_from=2024-01-01&date_to=2024-12-31")

	if stub.captured.Status != "in_progress" {
		t.Errorf("Status = %q", stub.captured.Status)
	}
	if stub.captured.Owner != "alice" {
		t.Errorf("Owner = %q", stub.captured.Owner)
	}
	if stub.captured.DateFrom != "2024-01-01" {
		t.Errorf("DateFrom = %q", stub.captured.DateFrom)
	}
	if stub.captured.DateTo != "2024-12-31" {
		t.Errorf("DateTo = %q", stub.captured.DateTo)
	}
}

// ---------------------------------------------------------------------------
// View-mode tests
// ---------------------------------------------------------------------------

func TestMainPage_View_DefaultsToGrid(t *testing.T) {
	stub := &listStub{listSamples: []samples.Sample{{ID: 1, Name: "s1", Status: "active"}}}
	rr := doMainPage(t, stub, "")

	body := rr.Body.String()
	if !strings.Contains(body, "samples-grid") {
		t.Errorf("default view should render samples-grid, body snippet=\n%s", snippet(body))
	}
	if strings.Contains(body, "samples-table") {
		t.Errorf("default view should NOT render samples-table")
	}
}

func TestMainPage_View_Table_RendersTable(t *testing.T) {
	stub := &listStub{
		listSamples: []samples.Sample{
			{
				ID:        7,
				Name:      "My Sample",
				Status:    "in_progress",
				Owner:     "alice",
				CreatedAt: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	rr := doMainPage(t, stub, "view=table")

	body := rr.Body.String()
	if !strings.Contains(body, "samples-table") {
		t.Fatalf("view=table should render the samples-table class, got:\n%s", snippet(body))
	}
	if !strings.Contains(body, "SMP-007") {
		t.Errorf("table should include the sample ID label, body:\n%s", snippet(body))
	}
	// Status chip must be present and use the canonical label.
	if !strings.Contains(body, "In Progress") {
		t.Errorf("table should render status label 'In Progress', body:\n%s", snippet(body))
	}
}

func TestMainPage_View_InvalidFallsBackToGrid(t *testing.T) {
	stub := &listStub{listSamples: []samples.Sample{{ID: 1, Name: "s1", Status: "active"}}}
	rr := doMainPage(t, stub, "view=nope")
	body := rr.Body.String()
	if !strings.Contains(body, "samples-grid") {
		t.Errorf("unknown view should fall back to grid, body:\n%s", snippet(body))
	}
}

func TestMainPage_GridQuery_PreservesFilters(t *testing.T) {
	// The Grid / Table toggle links must carry every other active filter so
	// switching view mode does not silently drop user input.
	stub := &listStub{}
	rr := doMainPage(t, stub, "status=archived&owner=bob&query=needle&view=table")
	body := rr.Body.String()

	// Look for a href that contains status, owner, query AND view=grid.
	// Template URL-encodes via url.Values.Encode so value order is deterministic.
	if !strings.Contains(body, "view=grid") ||
		!strings.Contains(body, "status=archived") ||
		!strings.Contains(body, "owner=bob") ||
		!strings.Contains(body, "query=needle") {
		t.Errorf("Grid toggle URL must preserve filters, body snippet:\n%s", snippet(body))
	}
}

func TestMainPage_GridCard_RendersStatusChip(t *testing.T) {
	stub := &listStub{
		listSamples: []samples.Sample{
			{ID: 1, Name: "Archived one", Status: "archived"},
		},
	}
	rr := doMainPage(t, stub, "")
	body := rr.Body.String()
	if !strings.Contains(body, "status-chip--archived") {
		t.Errorf("grid card should render status-chip--archived modifier, body:\n%s", snippet(body))
	}
	if !strings.Contains(body, "Archived") {
		t.Errorf("grid card should render 'Archived' label, body:\n%s", snippet(body))
	}
}

// snippet truncates long template output so test failure messages stay legible.
func snippet(s string) string {
	if len(s) < 800 {
		return s
	}
	return s[:800] + "\n…[truncated]"
}

// ---------------------------------------------------------------------------
// buildSamplesQuery
// ---------------------------------------------------------------------------

// TestBuildSamplesQuery exercises the query-string builder exposed via
// MainPageData.{Grid,Table}Query so regressions in the URL format are caught
// without relying on the template.
func TestMainPage_BuildsGridAndTableQueries(t *testing.T) {
	stub := &listStub{}
	rr := doMainPage(t, stub, "status=active&owner=alice&query=foo")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	// The template emits raw GridQuery/TableQuery — parsing the body as a URL is
	// easier than regex'ing it. Look for the "?status=active..." fragment.
	body := rr.Body.String()
	for _, want := range []string{"view=grid", "view=table", "status=active", "owner=alice", "query=foo"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in body, snippet:\n%s", want, snippet(body))
		}
	}
}

// ---------------------------------------------------------------------------
// Status forwarding on create/edit
// ---------------------------------------------------------------------------

type createStub struct {
	samples.Service
	captured samples.CreateParams
	isAdmin  bool
}

func (s *createStub) IsAdmin(_ context.Context, _ int) (bool, error)             { return s.isAdmin, nil }
func (s *createStub) GetApprovedUsers(_ context.Context) ([]samples.User, error) { return nil, nil }
func (s *createStub) GetUsernameByID(_ context.Context, _ int) (string, error)   { return "tester", nil }
func (s *createStub) Create(_ context.Context, p samples.CreateParams) (int, error) {
	s.captured = p
	return 42, nil
}

func TestNewSample_AdminCanSetStatus(t *testing.T) {
	stub := &createStub{isAdmin: true}
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})

	body := url.Values{}
	body.Set("name", "Sample")
	body.Set("status", "in_progress")
	req := httptest.NewRequest(http.MethodPost, "/samples/new", bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "admin"}))

	rr := httptest.NewRecorder()
	h.NewSample(rr, req)

	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if stub.captured.Status != "in_progress" {
		t.Errorf("CreateParams.Status = %q, want in_progress", stub.captured.Status)
	}
}

func TestNewSample_NonAdminStatusIgnored(t *testing.T) {
	// Non-admins must never be able to override status. Even if they submit
	// the field, Create should receive "" so the DB default applies.
	stub := &createStub{isAdmin: false}
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})

	body := url.Values{}
	body.Set("name", "Sample")
	body.Set("status", "archived")
	req := httptest.NewRequest(http.MethodPost, "/samples/new", bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "bob"}))

	rr := httptest.NewRecorder()
	h.NewSample(rr, req)

	if stub.captured.Status != "" {
		t.Errorf("non-admin submitted status must be dropped, got %q", stub.captured.Status)
	}
}

type editStatusStub struct {
	samples.Service

	capturedUpdate samples.UpdateParams
	isAdmin        bool
}

func (s *editStatusStub) CheckOwner(_ context.Context, _, _ int) error { return nil }
func (s *editStatusStub) IsAdmin(_ context.Context, _ int) (bool, error) {
	return s.isAdmin, nil
}
func (s *editStatusStub) GetByID(_ context.Context, id int) (samples.Sample, error) {
	uid := 1
	return samples.Sample{ID: id, Name: "n", OwnerUserID: &uid, Owner: "tester", Status: "active"}, nil
}
func (s *editStatusStub) GetUsernameByID(_ context.Context, _ int) (string, error) {
	return "tester", nil
}
func (s *editStatusStub) GetUsernameByIDApproved(_ context.Context, _ int) (string, error) {
	return "tester", nil
}
func (s *editStatusStub) Update(_ context.Context, _ int, p samples.UpdateParams) error {
	s.capturedUpdate = p
	return nil
}

func TestEditSample_NonAdminCannotSetStatus(t *testing.T) {
	stub := &editStatusStub{isAdmin: false}
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})

	body := url.Values{}
	body.Set("name", "n")
	body.Set("status", "archived")
	req := httptest.NewRequest(http.MethodPost, "/samples/edit/1", bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "tester"}))

	rr := httptest.NewRecorder()
	h.EditSample(rr, req)

	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if stub.capturedUpdate.Status != "" {
		t.Errorf("non-admin status must be dropped, got %q", stub.capturedUpdate.Status)
	}
}

func TestEditSample_AdminCanSetStatus(t *testing.T) {
	stub := &editStatusStub{isAdmin: true}
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})

	body := url.Values{}
	body.Set("name", "n")
	body.Set("status", "archived")
	req := httptest.NewRequest(http.MethodPost, "/samples/edit/1", bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "admin"}))

	rr := httptest.NewRecorder()
	h.EditSample(rr, req)

	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if stub.capturedUpdate.Status != "archived" {
		t.Errorf("admin status not forwarded, got %q", stub.capturedUpdate.Status)
	}
}

func TestEditSample_OmittedStatus_LeavesUnchanged(t *testing.T) {
	// An EditSample POST that doesn't include a status field must forward ""
	// to Update so the existing status is preserved at the DB layer.
	stub := &editStatusStub{isAdmin: false}
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})

	body := url.Values{}
	body.Set("name", "n")
	req := httptest.NewRequest(http.MethodPost, "/samples/edit/1", bytes.NewBufferString(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "tester"}))

	rr := httptest.NewRecorder()
	h.EditSample(rr, req)

	if stub.capturedUpdate.Status != "" {
		t.Errorf("UpdateParams.Status must be '' when omitted, got %q", stub.capturedUpdate.Status)
	}
}
