package dashboard_test

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/dashboard"
)

func TestDBService_KPIs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM samples`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(247))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM bookings`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(8))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM articles`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(42))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM users`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))

	svc := dashboard.NewService(mock)
	k, err := svc.KPIs(context.Background())
	if err != nil {
		t.Fatalf("KPIs: %v", err)
	}
	if k.TotalSamples != 247 || k.ActiveBookings != 8 || k.WikiArticles != 42 || k.PendingApprovals != 3 {
		t.Errorf("KPIs = %+v", k)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestDBService_RecentSamples(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(`ORDER BY s\.created_at DESC`).
		WithArgs(5).
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_owner",
			"username", "status", "created_at", "attachment_id",
		}).
			AddRow(1, "GaAs Wafer Batch 042", "desc", "alice", "alice", "active", now, int64(11)).
			AddRow(2, "NV-Center", "", "", nil, "in_progress", now.Add(-time.Hour), nil))

	svc := dashboard.NewService(mock)
	rows, err := svc.RecentSamples(context.Background(), 5)
	if err != nil {
		t.Fatalf("RecentSamples: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows; want 2", len(rows))
	}
	if rows[0].Name != "GaAs Wafer Batch 042" || rows[0].Status != "active" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
	if rows[0].DisplayOwner() != "alice" {
		t.Errorf("DisplayOwner = %q; want alice", rows[0].DisplayOwner())
	}
	if rows[1].DisplayOwner() != "Unassigned" {
		t.Errorf("DisplayOwner = %q; want Unassigned", rows[1].DisplayOwner())
	}
	if rows[0].FirstImageID == nil || *rows[0].FirstImageID != 11 {
		t.Errorf("FirstImageID = %v", rows[0].FirstImageID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestDBService_RecentSamples_DefaultLimit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	mock.ExpectQuery(`ORDER BY s\.created_at DESC`).
		WithArgs(5).
		WillReturnRows(pgxmock.NewRows([]string{
			"sample_id", "sample_name", "sample_description", "sample_owner",
			"username", "status", "created_at", "attachment_id",
		}))

	svc := dashboard.NewService(mock)
	if _, err := svc.RecentSamples(context.Background(), 0); err != nil {
		t.Fatalf("RecentSamples: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestDBService_TodayBookings(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	loc := time.UTC
	now := time.Date(2026, 4, 20, 12, 0, 0, 0, loc)
	dayStart := time.Date(2026, 4, 20, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	start := time.Date(2026, 4, 20, 9, 0, 0, 0, loc)
	end := time.Date(2026, 4, 20, 10, 30, 0, 0, loc)
	mock.ExpectQuery(`FROM bookings b`).
		WithArgs(dayStart, dayEnd, false, 0).
		WillReturnRows(pgxmock.NewRows([]string{
			"booking_id", "equipment_id", "name", "status", "user_id", "username",
			"start_time", "end_time", "purpose",
		}).AddRow(101, 7, "AFM Dimension Icon", "online", 3, "bob", start, end, "L.chen"))

	svc := dashboard.NewService(mock)
	rows, err := svc.TodayBookings(context.Background(), now, loc, 0, false)
	if err != nil {
		t.Fatalf("TodayBookings: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows; want 1", len(rows))
	}
	if rows[0].EquipmentName != "AFM Dimension Icon" || rows[0].Purpose != "L.chen" {
		t.Errorf("row = %+v", rows[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}
