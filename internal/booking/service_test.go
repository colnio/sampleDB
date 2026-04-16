package booking_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/auth"
	"sampleDB/internal/booking"
	"sampleDB/internal/web"
)

// testDBPool connects to a real Postgres instance for integration tests.
// Returns an error (rather than skipping) so callers can decide to skip.
func testDBPool(t *testing.T, dsn string) (*pgxpool.Pool, error) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, err
	}
	t.Cleanup(pool.Close)
	return pool, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newSvc(t *testing.T) (booking.Service, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return booking.NewService(mock, time.UTC), mock
}

// ---------------------------------------------------------------------------
// GetAllEquipment
// ---------------------------------------------------------------------------

func TestGetAllEquipment_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT equipment_id, name`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location"}).
			AddRow(1, "SEM", "Scanning electron microscope", "Lab A").
			AddRow(2, "AFM", "Atomic force microscope", "Lab B"))

	equipment, err := svc.GetAllEquipment(context.Background())
	if err != nil {
		t.Fatalf("GetAllEquipment: %v", err)
	}
	if len(equipment) != 2 {
		t.Errorf("expected 2 equipment, got %d", len(equipment))
	}
	if equipment[0].Name != "SEM" {
		t.Errorf("expected first equipment name %q, got %q", "SEM", equipment[0].Name)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetAllEquipment_EmptyResult(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT equipment_id, name`).
		WillReturnRows(pgxmock.NewRows([]string{"equipment_id", "name", "description", "location"}))

	equipment, err := svc.GetAllEquipment(context.Background())
	if err != nil {
		t.Fatalf("GetAllEquipment: %v", err)
	}
	if equipment != nil {
		t.Errorf("expected nil equipment, got %v", equipment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CheckUserPermission
// ---------------------------------------------------------------------------

func TestCheckUserPermission_Allowed(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(1, 2).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	ok, err := svc.CheckUserPermission(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("CheckUserPermission: %v", err)
	}
	if !ok {
		t.Error("expected permission granted")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestCheckUserPermission_Denied(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(1, 2).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	ok, err := svc.CheckUserPermission(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("CheckUserPermission: %v", err)
	}
	if ok {
		t.Error("expected permission denied")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// IsAdmin
// ---------------------------------------------------------------------------

func TestIsAdmin_True(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT COALESCE\(admin, false\) FROM users`).
		WithArgs(5).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	isAdmin, err := svc.IsAdmin(context.Background(), 5)
	if err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	if !isAdmin {
		t.Error("expected admin=true")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetBookingsForWeek — boundary test
// ---------------------------------------------------------------------------

// TestGetBookingsForWeek_SpansBoundary verifies that a booking overlapping a
// week boundary appears in the week view that contains either end of the booking.
// This mirrors the test previously in booking_boundary_test.go (package main)
// but now tests via the Service interface.
func TestGetBookingsForWeek_SpansBoundary(t *testing.T) {
	svc, mock := newSvc(t)

	tz := time.UTC
	weekEnd := time.Date(2024, 1, 7, 0, 0, 0, 0, tz)
	weekStart := weekEnd.AddDate(0, 0, -7)

	// Booking starts 2 h before the boundary, ends 10 h after it.
	bookingStart := weekEnd.Add(-2 * time.Hour)
	bookingEnd := weekEnd.Add(10 * time.Hour)
	now := time.Now()

	cols := []string{
		"booking_id", "equipment_id", "user_id", "username",
		"start_time", "end_time", "purpose", "created_at",
	}

	// The query must use the OVERLAPS condition: start_time < $2 AND end_time > $1
	mock.ExpectQuery(`b\.start_time < \$2 AND b\.end_time > \$1`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(mock.NewRows(cols).AddRow(
			1, 1, 1, "testuser",
			bookingStart, bookingEnd, "cross-boundary booking", now,
		))

	bookings, err := svc.GetBookingsForWeek(context.Background(), weekStart, weekEnd)
	if err != nil {
		t.Fatalf("GetBookingsForWeek returned error: %v", err)
	}
	if len(bookings) != 1 {
		t.Errorf("expected 1 boundary-spanning booking, got %d", len(bookings))
	}
	if len(bookings) == 1 && bookings[0].ID != 1 {
		t.Errorf("unexpected booking ID: got %d, want 1", bookings[0].ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

func TestGetBookingsForWeek_Empty(t *testing.T) {
	svc, mock := newSvc(t)

	cols := []string{
		"booking_id", "equipment_id", "user_id", "username",
		"start_time", "end_time", "purpose", "created_at",
	}
	mock.ExpectQuery(`b\.start_time < \$2 AND b\.end_time > \$1`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(mock.NewRows(cols))

	tz := time.UTC
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, tz)
	end := start.AddDate(0, 0, 7)

	bookings, err := svc.GetBookingsForWeek(context.Background(), start, end)
	if err != nil {
		t.Fatalf("GetBookingsForWeek: %v", err)
	}
	if len(bookings) != 0 {
		t.Errorf("expected empty slice, got %d bookings", len(bookings))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CheckConflict
// ---------------------------------------------------------------------------

func TestCheckConflict_HasConflict(t *testing.T) {
	svc, mock := newSvc(t)

	start := time.Date(2024, 1, 10, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(1, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	hasConflict, err := svc.CheckConflict(context.Background(), 1, start, end)
	if err != nil {
		t.Fatalf("CheckConflict: %v", err)
	}
	if !hasConflict {
		t.Error("expected conflict")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestCheckConflict_NoConflict(t *testing.T) {
	svc, mock := newSvc(t)

	start := time.Date(2024, 1, 10, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(1, pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	hasConflict, err := svc.CheckConflict(context.Background(), 1, start, end)
	if err != nil {
		t.Fatalf("CheckConflict: %v", err)
	}
	if hasConflict {
		t.Error("expected no conflict")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreate_Success(t *testing.T) {
	svc, mock := newSvc(t)

	start := time.Date(2024, 1, 10, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	mock.ExpectExec(`INSERT INTO bookings`).
		WithArgs(1, 2, pgxmock.AnyArg(), pgxmock.AnyArg(), "test purpose").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	err := svc.Create(context.Background(), booking.CreateParams{
		EquipmentID: 1,
		UserID:      2,
		StartTime:   start,
		EndTime:     end,
		Purpose:     "test purpose",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestCreate_ExclusionViolationReturnsErrConflict(t *testing.T) {
	svc, mock := newSvc(t)

	start := time.Date(2024, 1, 10, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	pgErr := &pgconn.PgError{Code: "23P01"}
	mock.ExpectExec(`INSERT INTO bookings`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgErr)

	err := svc.Create(context.Background(), booking.CreateParams{
		EquipmentID: 1,
		UserID:      2,
		StartTime:   start,
		EndTime:     end,
		Purpose:     "overlapping",
	})
	if !errors.Is(err, web.ErrConflict) {
		t.Errorf("expected web.ErrConflict for exclusion violation, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete_OwnerCanDelete(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT user_id FROM bookings WHERE booking_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(7))

	mock.ExpectExec(`DELETE FROM bookings WHERE booking_id`).
		WithArgs(42).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	if err := svc.Delete(context.Background(), 42, 7, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestDelete_NonOwnerReturnsForbidden(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT user_id FROM bookings WHERE booking_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(99))

	err := svc.Delete(context.Background(), 42, 7, false)
	if !errors.Is(err, web.ErrForbidden) {
		t.Errorf("expected web.ErrForbidden, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestDelete_AdminCanDeleteAnyBooking(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT user_id FROM bookings WHERE booking_id`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"user_id"}).AddRow(99))

	mock.ExpectExec(`DELETE FROM bookings WHERE booking_id`).
		WithArgs(42).
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	if err := svc.Delete(context.Background(), 42, 7, true); err != nil {
		t.Fatalf("admin Delete: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestDelete_MissingBookingReturnsNotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT user_id FROM bookings WHERE booking_id`).
		WithArgs(99).
		WillReturnError(pgx.ErrNoRows)

	err := svc.Delete(context.Background(), 99, 7, false)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Concurrency — verify exclusion constraint via service layer
// ---------------------------------------------------------------------------
// TestCreate_ConcurrentConflictViaService verifies that Create returns
// web.ErrConflict when the DB returns an exclusion_violation (code 23P01).
// This mirrors the intent of booking_conflict_test.go at the service layer.
func TestCreate_ConcurrentConflictViaService(t *testing.T) {
	svc, mock := newSvc(t)

	start := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	// First insert succeeds.
	mock.ExpectExec(`INSERT INTO bookings`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	// Second insert (concurrent) returns exclusion_violation.
	pgErr := &pgconn.PgError{Code: "23P01"}
	mock.ExpectExec(`INSERT INTO bookings`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(pgErr)

	p := booking.CreateParams{EquipmentID: 1, UserID: 2, StartTime: start, EndTime: end, Purpose: "concurrent"}

	if err := svc.Create(context.Background(), p); err != nil {
		t.Fatalf("first Create should succeed, got: %v", err)
	}
	err := svc.Create(context.Background(), p)
	if !errors.Is(err, web.ErrConflict) {
		t.Errorf("second Create should return web.ErrConflict, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

// mockService is a minimal mock of booking.Service for handler tests.
type mockService struct {
	equipment     []booking.Equipment
	bookings      []booking.Booking
	hasPermission bool
	isAdmin       bool
	createErr     error
	deleteErr     error
	conflictErr   error
	hasConflict   bool
}

func (m *mockService) GetAllEquipment(_ context.Context) ([]booking.Equipment, error) {
	return m.equipment, nil
}
func (m *mockService) CheckUserPermission(_ context.Context, _, _ int) (bool, error) {
	return m.hasPermission, nil
}
func (m *mockService) IsAdmin(_ context.Context, _ int) (bool, error) {
	return m.isAdmin, nil
}
func (m *mockService) GetBookingsForWeek(_ context.Context, _, _ time.Time) ([]booking.Booking, error) {
	return m.bookings, nil
}
func (m *mockService) CheckConflict(_ context.Context, _ int, _, _ time.Time) (bool, error) {
	return m.hasConflict, m.conflictErr
}
func (m *mockService) Create(_ context.Context, _ booking.CreateParams) error {
	return m.createErr
}
func (m *mockService) Delete(_ context.Context, _, _ int, _ bool) error {
	return m.deleteErr
}

// newAuthenticatedRequest creates a request with an auth session in its context.
func newAuthenticatedRequest(method, target string, body url.Values) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(body.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	session := auth.Session{UserID: 1, Username: "testuser"}
	ctx := auth.ContextWithSession(req.Context(), session)
	return req.WithContext(ctx)
}

// newRenderer builds a Renderer with an empty template set (suitable for handler
// tests that don't actually render HTML).
func newRenderer() *web.Renderer {
	return &web.Renderer{}
}

func TestHandleBooking_MethodNotAllowed(t *testing.T) {
	svc := &mockService{}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	req := newAuthenticatedRequest(http.MethodPut, "/booking", nil)
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestDeleteBooking_MethodNotAllowed(t *testing.T) {
	svc := &mockService{}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	req := newAuthenticatedRequest(http.MethodGet, "/booking/delete", nil)
	rr := httptest.NewRecorder()
	h.DeleteBooking(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestDeleteBooking_NonOwnerForbidden(t *testing.T) {
	svc := &mockService{deleteErr: web.ErrForbidden}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{"booking_id": {"42"}}
	req := newAuthenticatedRequest(http.MethodPost, "/booking/delete", form)
	rr := httptest.NewRecorder()
	h.DeleteBooking(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestDeleteBooking_NotFound(t *testing.T) {
	svc := &mockService{deleteErr: web.ErrNotFound}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{"booking_id": {"99"}}
	req := newAuthenticatedRequest(http.MethodPost, "/booking/delete", form)
	rr := httptest.NewRecorder()
	h.DeleteBooking(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDeleteBooking_Success(t *testing.T) {
	svc := &mockService{}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{"booking_id": {"1"}}
	req := newAuthenticatedRequest(http.MethodPost, "/booking/delete", form)
	rr := httptest.NewRecorder()
	h.DeleteBooking(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
}

func TestGetBookings_InvalidDate(t *testing.T) {
	svc := &mockService{}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	req := newAuthenticatedRequest(http.MethodGet, "/api/bookings?start=bad&end=bad", nil)
	rr := httptest.NewRecorder()
	h.GetBookings(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGetBookings_ReturnsJSON(t *testing.T) {
	svc := &mockService{bookings: []booking.Booking{}}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	req := newAuthenticatedRequest(http.MethodGet, "/api/bookings?start=2024-01-01&end=2024-01-08", nil)
	rr := httptest.NewRecorder()
	h.GetBookings(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestSubmitBooking_InvalidEquipmentID(t *testing.T) {
	svc := &mockService{}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{
		"equipment_id": {"not-an-int"},
		"start_time":   {"2024-01-10T09:00"},
		"end_time":     {"2024-01-10T10:00"},
		"purpose":      {"test"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/booking", form)
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "error") {
		t.Errorf("redirect should contain 'error', got %q", loc)
	}
}

func TestSubmitBooking_EndBeforeStart(t *testing.T) {
	svc := &mockService{}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{
		"equipment_id": {"1"},
		"start_time":   {"2024-01-10T10:00"},
		"end_time":     {"2024-01-10T09:00"},
		"purpose":      {"test"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/booking", form)
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "End+time") {
		t.Errorf("redirect should mention end time error, got %q", loc)
	}
}

func TestSubmitBooking_ConflictRedirects(t *testing.T) {
	svc := &mockService{hasConflict: true, isAdmin: true}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{
		"equipment_id": {"1"},
		"start_time":   {"2024-01-10T09:00"},
		"end_time":     {"2024-01-10T10:00"},
		"purpose":      {"test"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/booking", form)
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "already+booked") {
		t.Errorf("redirect should mention already booked, got %q", loc)
	}
}

func TestSubmitBooking_SuccessRedirects(t *testing.T) {
	svc := &mockService{isAdmin: true}
	h := booking.NewHandlers(svc, newRenderer(), nil, time.UTC)

	form := url.Values{
		"equipment_id": {"1"},
		"start_time":   {"2024-01-10T09:00"},
		"end_time":     {"2024-01-10T10:00"},
		"purpose":      {"test purpose"},
	}
	req := newAuthenticatedRequest(http.MethodPost, "/booking", form)
	rr := httptest.NewRecorder()
	h.HandleBooking(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "success") {
		t.Errorf("redirect should contain 'success', got %q", loc)
	}
}

// ---------------------------------------------------------------------------
// Integration — real DB concurrency test
// ---------------------------------------------------------------------------

// TestBookingExclusionConstraint_RealDB_ServiceLayer tests concurrent booking
// creation through the service layer against a real database.
// It is skipped when DATABASE_URL is not set.
func TestBookingExclusionConstraint_RealDB_ServiceLayer(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:app@localhost:5432/sampledb"
	}
	pool, err := testDBPool(t, dsn)
	if err != nil {
		t.Skipf("database not available (%v) — skipping integration test", err)
	}

	ctx := context.Background()

	// Ensure btree_gist and exclusion constraint are present.
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS btree_gist;`); err != nil {
		t.Fatalf("enable btree_gist: %v", err)
	}
	doSQL := `
DO $$ BEGIN
    ALTER TABLE bookings
        ADD CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (
            equipment_id WITH =,
            tstzrange(start_time, end_time, '[)') WITH &&
        );
EXCEPTION WHEN duplicate_object OR duplicate_table THEN NULL;
END $$;`
	if _, err := pool.Exec(ctx, doSQL); err != nil {
		t.Fatalf("add exclusion constraint: %v", err)
	}

	uname := fmt.Sprintf("bk-svc-test-user-%d", time.Now().UnixNano())
	var userID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, password_hash, is_approved) VALUES ($1, 'x', true) RETURNING user_id`,
		uname).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM bookings WHERE user_id = $1`, userID)
		pool.Exec(ctx, `DELETE FROM users WHERE user_id = $1`, userID)
	})

	ename := fmt.Sprintf("bk-svc-test-eq-%d", time.Now().UnixNano())
	var eqID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id`,
		ename).Scan(&eqID); err != nil {
		t.Fatalf("insert test equipment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM equipment WHERE equipment_id = $1`, eqID)
	})

	svc := booking.NewService(pool, time.UTC)
	start := time.Now().Add(time.Hour)
	end := start.Add(time.Hour)

	type result struct{ err error }
	results := make([]result, 2)
	var wg sync.WaitGroup

	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx].err = svc.Create(ctx, booking.CreateParams{
				EquipmentID: eqID,
				UserID:      userID,
				StartTime:   start,
				EndTime:     end,
				Purpose:     "concurrent test",
			})
		}(i)
	}
	wg.Wait()

	successes, conflicts := 0, 0
	for _, r := range results {
		if r.err == nil {
			successes++
		} else if errors.Is(r.err, web.ErrConflict) {
			conflicts++
		} else {
			t.Errorf("unexpected error (not ErrConflict): %v", r.err)
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 successful insert, got %d", successes)
	}
	if conflicts != 1 {
		t.Errorf("expected exactly 1 conflict, got %d", conflicts)
	}
}
