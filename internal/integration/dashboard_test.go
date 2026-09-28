//go:build integration

package integration_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// login posts credentials and returns the session cookie.
func login(t *testing.T, username, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "session_token" {
			return c
		}
	}
	t.Fatal("no session_token cookie after login")
	return nil
}

// TestRoot_RedirectsToDashboard verifies that `/` sends authenticated users
// to the new dashboard landing page (instead of the samples list it used to
// render).
func TestRoot_RedirectsToDashboard(t *testing.T) {
	username := "dashroot_" + randomHex(6)
	password := "dashroot_password1"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	var uid int
	if err := pool.QueryRow(bgCtx,
		"INSERT INTO users (username, password_hash, is_approved, email) VALUES ($1, $2, true, $3) RETURNING user_id",
		username, string(hash), username+"@example.com").Scan(&uid); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, "DELETE FROM users WHERE user_id = $1", uid) })

	cookie := login(t, username, password)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.AddCookie(cookie)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET / status = %d; want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/dashboard" {
		t.Errorf("Location = %q; want /dashboard", loc)
	}
}

// TestDashboard_Renders seeds a sample, a booking, and an activity entry,
// then hits /dashboard with an authenticated session and asserts that the
// aggregated data shows up in the response body.
func TestDashboard_Renders(t *testing.T) {
	ctx := context.Background()

	username := "dashrender_" + randomHex(6)
	password := "dashrender_password1"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	var uid int
	if err := pool.QueryRow(bgCtx,
		"INSERT INTO users (username, password_hash, is_approved, email) VALUES ($1, $2, true, $3) RETURNING user_id",
		username, string(hash), username+"@example.com").Scan(&uid); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, "DELETE FROM users WHERE user_id = $1", uid) })

	// Seed a sample owned by our new user.
	var sampleID int
	sampleName := "dash-integ-" + randomHex(4)
	if err := pool.QueryRow(ctx, `
		INSERT INTO samples (sample_name, owner_user_id, sample_owner)
		VALUES ($1, $2, $3) RETURNING sample_id`, sampleName, uid, username).Scan(&sampleID); err != nil {
		t.Fatalf("seed sample: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM samples WHERE sample_id = $1", sampleID) })

	// Seed a booking for today on any existing equipment.
	var equipmentID int
	if err := pool.QueryRow(ctx, `SELECT equipment_id FROM equipment ORDER BY equipment_id ASC LIMIT 1`).Scan(&equipmentID); err != nil {
		// Create one if none exist.
		if err := pool.QueryRow(ctx,
			`INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id`,
			"dash-integ-eq-"+randomHex(4)).Scan(&equipmentID); err != nil {
			t.Fatalf("seed equipment: %v", err)
		}
		t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM equipment WHERE equipment_id = $1", equipmentID) })
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 14, 0, 0, 0, time.Local)
	end := start.Add(time.Hour)
	var bookingID int
	if err := pool.QueryRow(ctx, `
		INSERT INTO bookings (equipment_id, user_id, start_time, end_time, purpose)
		VALUES ($1, $2, $3, $4, $5) RETURNING booking_id`,
		equipmentID, uid, start, end, "dash-integ-booking").Scan(&bookingID); err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM bookings WHERE booking_id = $1", bookingID) })

	// Seed an activity entry for the sample.
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_log (actor_user_id, actor_username, verb, target_type, target_id, target_label)
		VALUES ($1, $2, 'created', 'sample', $3, $4)`, uid, username, sampleID, sampleName); err != nil {
		t.Fatalf("seed activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM activity_log WHERE target_type='sample' AND target_id=$1`, sampleID)
	})

	cookie := login(t, username, password)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/dashboard", nil)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /dashboard: %v", err)
	}
	body := mustReadBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /dashboard status = %d; body=%s", resp.StatusCode, body)
	}
	for _, want := range []string{
		"Welcome back, " + username,
		sampleName,             // recent sample row
		"Total samples",        // KPI label
		"Today&#39;s bookings", // section heading (HTML-escaped apostrophe)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard body missing %q", want)
		}
	}
}
