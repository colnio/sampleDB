package main

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"
)

// TestGetBookingsForWeek_SpansBoundary verifies that a booking overlapping a
// week boundary appears in the week view that contains either end of the booking.
//
// The fix changes the WHERE clause from containment to overlap:
//
//	old: b.start_time >= $1 AND b.end_time <= $2
//	new: b.start_time < $2 AND b.end_time > $1
//
// This test is written against the overlap condition. If the old containment
// clause is still in place, pgxmock will not find a matching expectation and the
// test will fail.
func TestGetBookingsForWeek_SpansBoundary(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	// Ensure Location is set; TestMain initialises it to UTC but guard anyway.
	if theApp.Location == nil {
		theApp.Location = time.UTC
	}

	tz := time.UTC
	// weekEnd is midnight on Sunday — the boundary between two calendar weeks.
	weekEnd := time.Date(2024, 1, 7, 0, 0, 0, 0, tz)
	weekStart := weekEnd.AddDate(0, 0, -7)

	// Booking starts 2 h before the week boundary and ends 10 h after it —
	// it spans the boundary and should appear in the week-1 view.
	bookingStart := weekEnd.Add(-2 * time.Hour)
	bookingEnd := weekEnd.Add(10 * time.Hour)
	now := time.Now()

	cols := []string{
		"booking_id", "equipment_id", "user_id", "username",
		"start_time", "end_time", "purpose", "created_at",
	}

	// Expect a query that uses the OVERLAP condition.
	// The pattern will NOT match the old containment WHERE clause
	// (start_time >= $1 AND end_time <= $2), causing the test to fail before
	// the fix is applied.
	mock.ExpectQuery(`b\.start_time < \$2 AND b\.end_time > \$1`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(mock.NewRows(cols).AddRow(
			1, 1, 1, "testuser",
			bookingStart, bookingEnd, "cross-boundary booking", now,
		))

	bookings, err := getBookingsForWeek(context.Background(), weekStart, weekEnd)
	if err != nil {
		t.Fatalf("getBookingsForWeek returned error: %v", err)
	}
	if len(bookings) != 1 {
		t.Errorf("expected 1 boundary-spanning booking in week view, got %d", len(bookings))
	}
	if len(bookings) == 1 && bookings[0].ID != 1 {
		t.Errorf("unexpected booking ID: got %d, want 1", bookings[0].ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}
