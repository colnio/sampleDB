package booking_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/booking"
	"sampleDB/internal/web"
)

// renderStub is a minimal booking.Service used to render the booking calendar.
type renderStub struct {
	equipment []booking.Equipment
	isAdmin   bool
}

func (r *renderStub) GetAllEquipment(_ context.Context) ([]booking.Equipment, error) {
	return r.equipment, nil
}
func (r *renderStub) GetEquipmentStatus(_ context.Context, _ int) (string, error) {
	return "online", nil
}
func (r *renderStub) CheckUserPermission(_ context.Context, _, _ int) (bool, error) {
	return true, nil
}
func (r *renderStub) IsAdmin(_ context.Context, _ int) (bool, error) { return r.isAdmin, nil }
func (r *renderStub) GetBookingsForWeek(_ context.Context, _, _ time.Time) ([]booking.Booking, error) {
	return []booking.Booking{}, nil
}
func (r *renderStub) CheckConflict(_ context.Context, _ int, _, _ time.Time) (bool, error) {
	return false, nil
}
func (r *renderStub) Create(_ context.Context, _ booking.CreateParams) (int, error) {
	return 0, nil
}
func (r *renderStub) Delete(_ context.Context, _, _ int, _ bool) error { return nil }
func (r *renderStub) GetBooking(_ context.Context, _ int) (booking.Booking, error) {
	return booking.Booking{}, nil
}
func (r *renderStub) VerifyUserPassword(_ context.Context, _ int, _ string) (bool, error) {
	return true, nil
}

func renderBookingPage(t *testing.T, equipment []booking.Equipment) *httptest.ResponseRecorder {
	t.Helper()
	baseDir, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatalf("abs templates dir: %v", err)
	}
	r, err := web.NewRenderer(baseDir, nil, time.UTC)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	svc := &renderStub{equipment: equipment, isAdmin: true}
	h := booking.NewHandlers(svc, r, auth.NewManager(nil), time.UTC)

	req := httptest.NewRequest(http.MethodGet, "/booking", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "tester"}))
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)
	return rr
}

// TestBookingCalendar_RendersEquipmentStatusDot verifies that the booking page
// renders a per-equipment status dot using the equipment.Status value. The
// screenshot shows a coloured circle next to each equipment name; we assert
// that the specific CSS class is present for each equipment status the service
// returns.
func TestBookingCalendar_RendersEquipmentStatusDot(t *testing.T) {
	equipment := []booking.Equipment{
		{ID: 1, Name: "SEM-One", Description: "d", Location: "l", Status: "online"},
		{ID: 2, Name: "AFM-Two", Description: "d", Location: "l", Status: "maintenance"},
		{ID: 3, Name: "Probe-Three", Description: "d", Location: "l", Status: "offline"},
	}
	rr := renderBookingPage(t, equipment)
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()

	wants := []string{
		"tline-row--online",
		"tline-row--maintenance",
		"tline-row--offline",
		"SEM-One",
		"AFM-Two",
		"Probe-Three",
		"Book Slot",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("booking page render missing %q", w)
		}
	}
}

// TestBookingCalendar_RulesStubRemoved guards against regression of QA UX-4:
// the commented-out "Rules" sidebar stub must not reappear in the rendered
// output.
func TestBookingCalendar_RulesStubRemoved(t *testing.T) {
	rr := renderBookingPage(t, []booking.Equipment{
		{ID: 1, Name: "SEM", Status: "online"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d", rr.Code)
	}
	body := rr.Body.String()
	// The stub was a sidebar link text; it must not appear anywhere.
	if strings.Contains(body, "Booking Rules") || strings.Contains(body, "\"Rules\"") {
		t.Errorf("booking page still contains a rules stub")
	}
}

func TestBookingCalendar_UsesScopedModalClasses(t *testing.T) {
	rr := renderBookingPage(t, []booking.Equipment{
		{ID: 1, Name: "SEM", Status: "online"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="booking-modal"`) {
		t.Fatalf("booking modal element not found")
	}
}
