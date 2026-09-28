//go:build integration

package integration_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var csrfInputRe = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

// TestEmailGate_RequiresEmailAfterLogin walks the mandatory-email flow: a user
// without an email is sent to /profile/email from every page, and once they
// save one they continue to where they were going.
func TestEmailGate_RequiresEmailAfterLogin(t *testing.T) {
	username := "emailgate_" + randomHex(6)
	password := "emailgate_password1"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	var uid int
	if err := pool.QueryRow(bgCtx,
		"INSERT INTO users (username, password_hash, is_approved) VALUES ($1, $2, true) RETURNING user_id",
		username, string(hash)).Scan(&uid); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, "DELETE FROM users WHERE user_id = $1", uid) })

	cookie := login(t, username, password)

	// 1. Any page redirects to the email prompt, preserving the destination.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/booking", nil)
	req.AddCookie(cookie)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET /booking: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /booking status = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/profile/email?next=%2Fbooking" {
		t.Fatalf("Location = %q", loc)
	}

	// 2. The prompt renders and carries a CSRF token.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/profile/email?next=%2Fbooking", nil)
	req.AddCookie(cookie)
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET /profile/email: %v", err)
	}
	body := mustReadBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /profile/email status = %d", resp.StatusCode)
	}
	m := csrfInputRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("email page has no csrf_token input")
	}

	// 3. A duplicate (case-insensitive) email of the fixture admin is rejected.
	form := url.Values{"csrf_token": {m[1]}, "email": {strings.ToUpper(testUser.username) + "@EXAMPLE.COM"}, "next": {"/booking"}}
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/profile/email", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST duplicate email: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate email status = %d, want 409", resp.StatusCode)
	}

	// 4. Saving a fresh email continues to the original destination.
	form.Set("email", username+"@example.org")
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/profile/email", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST email: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/booking" {
		t.Fatalf("POST email = %d %q, want 303 /booking", resp.StatusCode, resp.Header.Get("Location"))
	}

	// 5. The gate is now open.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/booking", nil)
	req.AddCookie(cookie)
	resp, err = noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("GET /booking after email: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /booking after email status = %d, want 200", resp.StatusCode)
	}
}
