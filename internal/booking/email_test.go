package booking_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
	"sampleDB/internal/booking"
	"sampleDB/internal/web"
)

func renderBookingPageAs(t *testing.T, svc *renderStub, userID int) string {
	t.Helper()
	baseDir, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatalf("abs templates dir: %v", err)
	}
	r, err := web.NewRenderer(baseDir, nil, time.UTC)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	h := booking.NewHandlers(svc, r, auth.NewManager(nil), time.UTC)
	req := httptest.NewRequest(http.MethodGet, "/booking", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: userID, Username: "viewer"}))
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d, body = %s", rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

func emailTestStub(isAdmin bool) *renderStub {
	now := time.Now()
	return &renderStub{
		isAdmin:   isAdmin,
		equipment: []booking.Equipment{{ID: 1, Name: "SEM", Status: "online"}},
		bookings: []booking.Booking{
			{ID: 10, EquipmentID: 1, UserID: 2, Username: "bob", StartTime: now, EndTime: now.Add(time.Hour)},
			{ID: 11, EquipmentID: 1, UserID: 2, Username: "bob", StartTime: now.Add(2 * time.Hour), EndTime: now.Add(3 * time.Hour)},
			{ID: 12, EquipmentID: 1, UserID: 1, Username: "viewer", StartTime: now.Add(4 * time.Hour), EndTime: now.Add(5 * time.Hour)},
		},
		emails: map[int]string{1: "viewer@example.com", 2: "bob@example.com"},
	}
}

func TestBookingCalendar_AdminGetsOwnerEmails(t *testing.T) {
	svc := emailTestStub(true)
	body := renderBookingPageAs(t, svc, 1)

	if !strings.Contains(body, "bob@example.com") {
		t.Error("admin page should carry the booking owner's email")
	}
	if !strings.Contains(body, `id="del-admin-email"`) {
		t.Error("admin delete modal missing email link")
	}
	// One lookup, de-duplicated, excluding the admin's own bookings.
	if len(svc.emailLookups) != 1 || len(svc.emailLookups[0]) != 1 || svc.emailLookups[0][0] != 2 {
		t.Errorf("GetUserEmails calls = %v, want [[2]]", svc.emailLookups)
	}
}

func TestBookingCalendar_NonAdminNeverGetsEmails(t *testing.T) {
	svc := emailTestStub(false)
	body := renderBookingPageAs(t, svc, 1)

	if strings.Contains(body, "@example.com") {
		t.Error("non-admin page must not contain any email address")
	}
	if len(svc.emailLookups) != 0 {
		t.Errorf("GetUserEmails must not be called for non-admins, got %v", svc.emailLookups)
	}
}

func TestGetUserEmails_Query(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	svc := booking.NewService(mock, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT user_id, email FROM users WHERE user_id = ANY($1) AND email IS NOT NULL`)).
		WithArgs([]int{2, 3}).
		WillReturnRows(pgxmock.NewRows([]string{"user_id", "email"}).AddRow(2, "bob@example.com"))

	got, err := svc.GetUserEmails(context.Background(), []int{2, 3})
	if err != nil {
		t.Fatalf("GetUserEmails: %v", err)
	}
	if len(got) != 1 || got[2] != "bob@example.com" {
		t.Errorf("got %v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetUserEmails_EmptyInputSkipsQuery(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()
	svc := booking.NewService(mock, time.UTC)

	got, err := svc.GetUserEmails(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
