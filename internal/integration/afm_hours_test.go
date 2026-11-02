//go:build integration

package integration_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/auth"
)

// TestBookings_AFMWorkingHours drives the feature end to end against a real
// database. Dates are in 2099, where the 15th, 22nd and 29th of March are Sundays
// and the 16th and 23rd are Mondays.
func TestBookings_AFMWorkingHours(t *testing.T) {
	var equipID int
	if err := pool.QueryRow(bgCtx,
		"INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id",
		"integ-AFM-"+randomHex(4)).Scan(&equipID); err != nil {
		t.Fatalf("seed equipment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(bgCtx, "DELETE FROM bookings WHERE equipment_id = $1", equipID)
		pool.Exec(bgCtx, "DELETE FROM user_equipment_permissions WHERE equipment_id = $1", equipID)
		pool.Exec(bgCtx, "DELETE FROM equipment WHERE equipment_id = $1", equipID)
		pool.Exec(bgCtx, "UPDATE users SET afm_outside_hours_allowed = true WHERE user_id = $1", testUser.id)
		pool.Exec(bgCtx, "UPDATE working_hours SET start_minute = 480, end_minute = 1080, weekdays = '{1,2,3,4,5}'")
		pool.Exec(bgCtx, "DELETE FROM working_hours_exceptions WHERE exception_date >= '2099-01-01'")
	})

	postForm := func(path string, form url.Values) string {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+apiToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("POST %s status = %d, want 303", path, resp.StatusCode)
		}
		return resp.Header.Get("Location")
	}
	book := func(start, end string) string {
		t.Helper()
		return postForm("/booking", url.Values{
			"equipment_id": {fmt.Sprint(equipID)},
			"start_time":   {start},
			"end_time":     {end},
			"purpose":      {"AFM hours test"},
		})
	}
	setAccess := func(extra string) {
		t.Helper()
		body := fmt.Sprintf(`{"user_id":%d,"approved":true%s}`, testUser.id, extra)
		resp := apiDo(t, http.MethodPost, "/admin/update-access", jsonBody(body))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("update-access status = %d", resp.StatusCode)
		}
	}
	wantBooked := func(what, start, end string) {
		t.Helper()
		if loc := book(start, end); !strings.Contains(loc, "success=") {
			t.Errorf("%s: Location = %q, want success", what, loc)
		}
	}
	wantBlocked := func(what, start, end string) string {
		t.Helper()
		loc := book(start, end)
		if !strings.Contains(loc, "error=") {
			t.Errorf("%s: Location = %q, want error", what, loc)
		}
		return loc
	}

	// Every user starts unrestricted, including outside the default hours.
	wantBooked("default user on a Sunday", "2099-03-15T10:00", "2099-03-15T11:00")

	// Unticked: limited to Mon-Fri 08:00-18:00.
	setAccess(`,"afm_outside_hours_allowed":false`)
	wantBlocked("weekend", "2099-03-22T10:00", "2099-03-22T11:00")
	wantBlocked("crosses closing", "2099-03-16T17:00", "2099-03-16T19:00")
	wantBooked("full working day", "2099-03-16T08:00", "2099-03-16T18:00")

	// Omitting the field must not change it.
	setAccess("")
	wantBlocked("after update without the flag", "2099-03-22T10:00", "2099-03-22T11:00")

	// The schedule is adjustable: Sundays 09:00-12:00 only.
	if loc := postForm("/admin/working-hours", url.Values{"start": {"09:00"}, "end": {"12:00"}, "weekday": {"0"}}); !strings.Contains(loc, "success=") {
		t.Fatalf("set working hours: Location = %q", loc)
	}
	wantBooked("new working day", "2099-03-22T10:00", "2099-03-22T11:00")
	wantBlocked("after new closing", "2099-03-22T11:00", "2099-03-22T13:00")
	wantBlocked("old working day", "2099-03-23T09:00", "2099-03-23T10:00")

	// An exception closes a working day, and removing it reopens the day.
	addException := func(form url.Values) {
		t.Helper()
		if loc := postForm("/admin/working-hours/exceptions", form); !strings.Contains(loc, "success=") {
			t.Fatalf("add exception: Location = %q", loc)
		}
	}
	addException(url.Values{"date": {"2099-03-29"}, "note": {"Holiday"}})
	if loc := wantBlocked("holiday", "2099-03-29T10:00", "2099-03-29T11:00"); !strings.Contains(loc, "Holiday") {
		t.Errorf("Location = %q, want the note", loc)
	}
	if loc := postForm("/admin/working-hours/exceptions/delete", url.Values{"date": {"2099-03-29"}}); !strings.Contains(loc, "success=") {
		t.Fatalf("delete exception: Location = %q", loc)
	}
	wantBooked("after removing the holiday", "2099-03-29T10:00", "2099-03-29T11:00")

	// An exception can also open a day the schedule doesn't, with its own hours,
	// and saving the same date again replaces it.
	addException(url.Values{"date": {"2099-03-30"}, "start": {"13:00"}, "end": {"14:00"}})
	addException(url.Values{"date": {"2099-03-30"}, "start": {"13:00"}, "end": {"15:00"}})
	wantBooked("inside replaced exception", "2099-03-30T14:00", "2099-03-30T15:00")
	wantBlocked("outside exception", "2099-03-30T12:00", "2099-03-30T13:00")

	// Ticking the box again lifts the limit.
	setAccess(`,"afm_outside_hours_allowed":true`)
	wantBooked("re-ticked", "2099-03-31T23:00", "2099-03-31T23:30")
}

// The working-hours routes must stay behind both the admin and CSRF checks.
func TestWorkingHoursRoutes_AreGuarded(t *testing.T) {
	const password = "guardtest_password1"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	createUser := func(isAdmin bool) (id int, username string) {
		t.Helper()
		username = "guardtest_" + randomHex(6)
		if err := pool.QueryRow(bgCtx,
			"INSERT INTO users (username, password_hash, is_approved, admin, email) VALUES ($1, $2, true, $3, $4) RETURNING user_id",
			username, string(hash), isAdmin, username+"@example.com").Scan(&id); err != nil {
			t.Fatalf("create user: %v", err)
		}
		t.Cleanup(func() { pool.Exec(bgCtx, "DELETE FROM users WHERE user_id = $1", id) })
		return id, username
	}
	var before string
	schedule := func() string {
		t.Helper()
		var got string
		if err := pool.QueryRow(bgCtx,
			"SELECT start_minute || '-' || end_minute FROM working_hours").Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	before = schedule()

	routes := map[string]url.Values{
		"/admin/working-hours":                   {"start": {"01:00"}, "end": {"02:00"}, "weekday": {"1"}},
		"/admin/working-hours/exceptions":        {"date": {"2099-08-01"}},
		"/admin/working-hours/exceptions/delete": {"date": {"2099-08-01"}},
	}

	// A non-admin is turned away, even with a CSRF-exempt bearer token.
	userID, _ := createUser(false)
	plain, tokenHash, err := auth.GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(bgCtx,
		"INSERT INTO api_tokens (user_id, token_hash, name) VALUES ($1, $2, 'guard-test')", userID, tokenHash); err != nil {
		t.Fatal(err)
	}
	for path, form := range routes {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Authorization", "Bearer "+plain)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if loc := resp.Header.Get("Location"); resp.StatusCode == http.StatusOK || strings.HasPrefix(loc, "/admin") {
			t.Errorf("non-admin POST %s: status %d, Location %q", path, resp.StatusCode, loc)
		}
	}

	// A browser session of an admin needs a CSRF token.
	_, adminName := createUser(true)
	login := url.Values{"username": {adminName}, "password": {password}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader(login.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		t.Fatalf("login set no cookie, status %d", resp.StatusCode)
	}
	for path, form := range routes {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, c := range cookies {
			req.AddCookie(c)
		}
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("POST %s without CSRF token: status %d, want 403", path, resp.StatusCode)
		}
	}

	if got := schedule(); got != before {
		t.Errorf("schedule changed from %s to %s", before, got)
	}
	var exceptions int
	if err := pool.QueryRow(bgCtx,
		"SELECT count(*) FROM working_hours_exceptions WHERE exception_date = '2099-08-01'").Scan(&exceptions); err != nil {
		t.Fatal(err)
	}
	if exceptions != 0 {
		t.Error("a guarded request created an exception")
	}
}
