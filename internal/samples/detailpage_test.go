package samples_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/activity"
	"sampleDB/internal/auth"
	"sampleDB/internal/samples"
)

// detailStub is a minimal Service stub used by the sample-detail handler tests.
// It embeds samples.Service so only the methods used by the detail path are
// implemented; any other call will panic, making missing coverage obvious.
type detailStub struct {
	samples.Service

	sample    samples.Sample
	users     []samples.User
	equipment []samples.Equipment
	steps     []samples.FabricationStep
	isAdmin   bool
	getErr    error
}

func (s *detailStub) GetByID(_ context.Context, _ int) (samples.Sample, error) {
	return s.sample, s.getErr
}
func (s *detailStub) GetApprovedUsers(_ context.Context) ([]samples.User, error) {
	return s.users, nil
}
func (s *detailStub) GetAllEquipment(_ context.Context) ([]samples.Equipment, error) {
	return s.equipment, nil
}
func (s *detailStub) GetSteps(_ context.Context, _ int) ([]samples.FabricationStep, error) {
	return s.steps, nil
}
func (s *detailStub) IsAdmin(_ context.Context, _ int) (bool, error) {
	return s.isAdmin, nil
}

func newDetailHandlers(t *testing.T, stub samples.Service, logger *activity.Logger) *samples.Handlers {
	t.Helper()
	h := samples.NewHandlers(stub, newRenderer(t), auth.NewManager(nil), samples.HandlerConfig{})
	h.Activity = logger
	return h
}

func doSampleDetail(t *testing.T, h *samples.Handlers) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/samples/1", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 7, Username: "alice"}))
	rr := httptest.NewRecorder()
	h.HandleSample(rr, req)
	return rr
}

// TestSampleDetail_ActivityPanel_RendersEntries verifies that activity entries
// returned by the Logger are passed into the template context and rendered in
// the Activity card.
func TestSampleDetail_ActivityPanel_RendersEntries(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	now := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	actorID := int64(99)
	targetID := int64(1)
	mock.ExpectQuery(`SELECT id, actor_user_id`).
		WithArgs("sample", 1, 6).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "actor_user_id", "actor_username", "verb", "target_type", "target_id", "target_label", "created_at",
		}).
			AddRow(int64(2), actorID, "bob", "updated", "sample", targetID, "widget", now).
			AddRow(int64(1), actorID, "bob", "created", "sample", targetID, "widget", now.Add(-time.Hour)))

	logger := activity.NewLogger(mock)

	stub := &detailStub{
		sample: samples.Sample{
			ID:        1,
			Name:      "widget",
			Status:    "active",
			CreatedAt: now,
		},
	}
	h := newDetailHandlers(t, stub, logger)

	rr := doSampleDetail(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Activity") {
		t.Errorf("Activity card heading not rendered")
	}
	if !strings.Contains(body, "bob") {
		t.Errorf("Activity entry actor not rendered; body=%s", body)
	}
	// At least one of the activityVerb expansions should appear.
	if !strings.Contains(body, "edited") && !strings.Contains(body, "created") {
		t.Errorf("Activity entry verb not rendered")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSampleDetail_ActivityPanel_EmptyWhenNoEntries verifies the "No activity
// yet" empty state renders when the logger returns no rows.
func TestSampleDetail_ActivityPanel_EmptyWhenNoEntries(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery(`SELECT id, actor_user_id`).
		WithArgs("sample", 1, 6).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "actor_user_id", "actor_username", "verb", "target_type", "target_id", "target_label", "created_at",
		}))

	logger := activity.NewLogger(mock)
	stub := &detailStub{
		sample: samples.Sample{ID: 1, Name: "widget", Status: "active", CreatedAt: time.Now().UTC()},
	}
	h := newDetailHandlers(t, stub, logger)

	rr := doSampleDetail(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "No activity yet") {
		t.Errorf("empty-state text missing; body=%s", rr.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestSampleDetail_NilLogger_DoesNotCrash verifies that a nil Activity logger
// still produces a rendered page (logger is optional best-effort infrastructure).
func TestSampleDetail_NilLogger_DoesNotCrash(t *testing.T) {
	stub := &detailStub{
		sample: samples.Sample{ID: 1, Name: "widget", Status: "active", CreatedAt: time.Now().UTC()},
	}
	h := newDetailHandlers(t, stub, nil)

	rr := doSampleDetail(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "No activity yet") {
		t.Errorf("empty-state text should render when logger is nil; body=%s", rr.Body.String())
	}
}

// TestSampleDetail_StatusChip_Active verifies the page header status chip
// renders with the 'active' modifier class and label.
func TestSampleDetail_StatusChip_Active(t *testing.T) {
	stub := &detailStub{
		sample: samples.Sample{ID: 1, Name: "widget", Status: "active", CreatedAt: time.Now().UTC()},
	}
	h := newDetailHandlers(t, stub, nil)
	rr := doSampleDetail(t, h)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "status-chip--active") {
		t.Errorf("expected status-chip--active class; body=%s", body)
	}
	if !strings.Contains(body, ">Active<") {
		t.Errorf("expected 'Active' label inside chip; body=%s", body)
	}
}

// TestSampleDetail_StatusChip_NonActive verifies the chip still renders for
// non-active statuses (this is the assertion called out in the plan: chip must
// appear for in_progress / archived samples as well).
func TestSampleDetail_StatusChip_NonActive(t *testing.T) {
	cases := []struct {
		status    string
		wantClass string
		wantLabel string
	}{
		{"in_progress", "status-chip--progress", ">In Progress<"},
		{"archived", "status-chip--archived", ">Archived<"},
	}
	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			stub := &detailStub{
				sample: samples.Sample{ID: 1, Name: "widget", Status: tc.status, CreatedAt: time.Now().UTC()},
			}
			h := newDetailHandlers(t, stub, nil)
			rr := doSampleDetail(t, h)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d", rr.Code)
			}
			body := rr.Body.String()
			if !strings.Contains(body, tc.wantClass) {
				t.Errorf("missing class %s; body=%s", tc.wantClass, body)
			}
			if !strings.Contains(body, tc.wantLabel) {
				t.Errorf("missing label %s; body=%s", tc.wantLabel, body)
			}
		})
	}
}

// TestSampleDetail_DangerZone_AdminOnly verifies the Danger Zone card renders
// only when the session user is an admin.
func TestSampleDetail_DangerZone_AdminOnly(t *testing.T) {
	t.Run("admin sees danger zone", func(t *testing.T) {
		stub := &detailStub{
			sample:  samples.Sample{ID: 1, Name: "widget", Status: "active", CreatedAt: time.Now().UTC()},
			isAdmin: true,
		}
		h := newDetailHandlers(t, stub, nil)
		rr := doSampleDetail(t, h)
		if !strings.Contains(rr.Body.String(), "Danger Zone") {
			t.Errorf("admin should see Danger Zone; body=%s", rr.Body.String())
		}
	})
	t.Run("non-admin does not see danger zone", func(t *testing.T) {
		stub := &detailStub{
			sample:  samples.Sample{ID: 1, Name: "widget", Status: "active", CreatedAt: time.Now().UTC()},
			isAdmin: false,
		}
		h := newDetailHandlers(t, stub, nil)
		rr := doSampleDetail(t, h)
		if strings.Contains(rr.Body.String(), "Danger Zone") {
			t.Errorf("non-admin should not see Danger Zone; body=%s", rr.Body.String())
		}
	})
}
