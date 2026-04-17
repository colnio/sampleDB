//go:build integration

package integration_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestAuth_LoginLogout(t *testing.T) {
	username := "authtest_" + randomHex(6)
	password := "authtest_password1"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	var uid int
	if err := pool.QueryRow(bgCtx,
		"INSERT INTO users (username, password_hash, is_approved) VALUES ($1, $2, true) RETURNING user_id",
		username, string(hash)).Scan(&uid); err != nil {
		t.Fatalf("create auth test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(bgCtx, "DELETE FROM users WHERE user_id = $1", uid)
	})

	// Login — expect 303 redirect with session cookie.
	form := url.Values{"username": {username}, "password": {password}}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", resp.StatusCode)
	}

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session_token" {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session_token cookie in login response")
	}

	// Logout using the session cookie.
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/logout", nil)
	req2.AddCookie(sessionCookie)
	resp2, err := noRedirectClient.Do(req2)
	if err != nil {
		t.Fatalf("GET /logout: %v", err)
	}
	resp2.Body.Close()

	if resp2.StatusCode != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want 303", resp2.StatusCode)
	}
	loc := resp2.Header.Get("Location")
	if !strings.Contains(loc, "login") {
		t.Fatalf("logout redirect Location = %q, want /login", loc)
	}
}

func TestAuth_Register(t *testing.T) {
	username := "regtest_" + randomHex(6)
	form := url.Values{
		"username":         {username},
		"password":         {"registerpass1"},
		"confirm_password": {"registerpass1"},
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST /register: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("register status = %d, want 303", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "success") {
		t.Fatalf("register redirect = %q, want success message", loc)
	}

	// Clean up the newly registered user.
	pool.Exec(bgCtx, "DELETE FROM users WHERE username = $1", username)
}

func TestAuth_APIRequiresToken(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/samples", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-auth API status = %d, want 401", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("no-auth Content-Type = %q, want application/json", ct)
	}
}
