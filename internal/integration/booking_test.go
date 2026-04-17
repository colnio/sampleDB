//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/booking"
)

func TestBookings_ListEmpty(t *testing.T) {
	// Use a date far in the future so we don't collide with real bookings.
	start := "2099-06-01"
	end := "2099-06-08"
	resp := apiDo(t, http.MethodGet, fmt.Sprintf("/api/bookings?start=%s&end=%s", start, end), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/bookings status = %d, want 200", resp.StatusCode)
	}

	var bookings []booking.Booking
	if err := json.NewDecoder(resp.Body).Decode(&bookings); err != nil {
		t.Fatalf("decode bookings: %v", err)
	}
	// Result may be empty or have items — just confirm JSON is valid.
}

func TestBookings_CreateAndList(t *testing.T) {
	// Find an existing equipment_id to book against.
	var equipID int
	if err := pool.QueryRow(bgCtx, "SELECT equipment_id FROM equipment LIMIT 1").Scan(&equipID); err != nil {
		t.Skip("no equipment in database; skipping booking test")
	}

	// Use a date far in the future and a unique time window to avoid conflicts.
	loc := time.UTC
	start := time.Date(2099, 3, 15, 10, 0, 0, 0, loc)
	end := time.Date(2099, 3, 15, 11, 0, 0, 0, loc)
	startStr := start.Format("2006-01-02T15:04")
	endStr := end.Format("2006-01-02T15:04")

	form := url.Values{
		"equipment_id": {fmt.Sprint(equipID)},
		"start_time":   {startStr},
		"end_time":     {endStr},
		"purpose":      {"Integration test booking"},
	}

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/booking", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("build booking request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST /booking: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /booking status = %d, want 303", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.Contains(location, "success") {
		t.Fatalf("expected success redirect, got Location = %q", location)
	}

	// Look up the booking_id we just created so we can clean it up.
	var bookingID int
	err = pool.QueryRow(bgCtx,
		"SELECT booking_id FROM bookings WHERE equipment_id = $1 AND user_id = $2 AND purpose = 'Integration test booking' ORDER BY booking_id DESC LIMIT 1",
		equipID, testUser.id).Scan(&bookingID)
	if err != nil {
		t.Logf("could not find booking row for cleanup: %v", err)
	} else {
		t.Cleanup(func() {
			pool.Exec(bgCtx, "DELETE FROM bookings WHERE booking_id = $1", bookingID)
		})
	}

	// List bookings for the created window.
	listStart := start.Format("2006-01-02")
	listEnd := end.AddDate(0, 0, 1).Format("2006-01-02")
	resp2 := apiDo(t, http.MethodGet, fmt.Sprintf("/api/bookings?start=%s&end=%s", listStart, listEnd), nil)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/bookings status = %d, want 200", resp2.StatusCode)
	}

	var bookings []booking.Booking
	if err := json.NewDecoder(resp2.Body).Decode(&bookings); err != nil {
		t.Fatalf("decode bookings list: %v", err)
	}
	found := false
	for _, b := range bookings {
		if b.UserID == testUser.id && b.EquipmentID == equipID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("created booking not found in list; got %d bookings", len(bookings))
	}
}
