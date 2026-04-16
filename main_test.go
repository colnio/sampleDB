package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
)

// TestMain initialises theApp with sensible defaults before any test in
// package main runs.  Individual tests that need a real DB or a specific mock
// override theApp.DB (or theApp.Auth) via setMockDB / their own setup.
func TestMain(m *testing.M) {
	theApp = &app.App{
		Location: time.UTC,
		// DB and Auth start nil; setMockDB / real-DB helpers will set them.
		Auth: auth.NewManager(nil),
	}
	os.Exit(m.Run())
}

func TestSanitizeFilename(t *testing.T) {
	tests := map[string]string{
		"../../foo bar?.txt": "foo-bar.txt",
		"   .hidden  ":       "file.hidden",
		"":                   "file",
	}

	for input, want := range tests {
		if got := sanitizeFilename(input); got != want {
			t.Fatalf("sanitizeFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestGenerateUniqueFilename(t *testing.T) {
	name, err := generateUniqueFilename(" My Image.PNG ")
	if err != nil {
		t.Fatalf("generateUniqueFilename returned error: %v", err)
	}

	re := regexp.MustCompile(`^[0-9a-f]{16}_My-Image\.png$`)
	if !re.MatchString(name) {
		t.Fatalf("generated filename %q does not match %q", name, re.String())
	}
}

func TestSetDownloadHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	setDownloadHeaders(rr, `report "1".txt`)

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="report 1.txt"`) {
		t.Fatalf("content disposition missing safe filename: %q", cd)
	}
	if !strings.Contains(cd, "filename*=UTF-8''report%201.txt") {
		t.Fatalf("content disposition missing encoded filename: %q", cd)
	}
}

func TestRedirectToHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.org/foo?bar=baz", nil)
	rr := httptest.NewRecorder()

	redirectToHTTPS("example.org", "8443").ServeHTTP(rr, req)

	if rr.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected permanent redirect, got %d", rr.Code)
	}

	if loc := rr.Header().Get("Location"); loc != "https://example.org:8443/foo?bar=baz" {
		t.Fatalf("unexpected redirect location: %q", loc)
	}
}

func TestSecurityHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}), true)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("unexpected status: %d", rr.Code)
	}

	headers := rr.Header()
	if headers.Get("X-Content-Type-Options") != "nosniff" ||
		headers.Get("X-Frame-Options") != "DENY" ||
		headers.Get("Referrer-Policy") != "no-referrer-when-downgrade" ||
		headers.Get("Strict-Transport-Security") == "" {
		t.Fatalf("security headers missing or incomplete: %+v", headers)
	}
}
