package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

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

func TestOriginalFilenameFromPath(t *testing.T) {
	got := originalFilenameFromPath("uploads/abcdef0123456789_notes.txt")
	if got != "notes.txt" {
		t.Fatalf("originalFilenameFromPath returned %q, want %q", got, "notes.txt")
	}
}

func TestSetDownloadHeaders(t *testing.T) {
	rr := httptest.NewRecorder()
	setDownloadHeaders(rr, `report "1".txt`)

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `filename="report 1.txt"`) {
		t.Fatalf("content disposition missing safe filename: %q", cd)
	}
	if !strings.Contains(cd, "filename*=UTF-8''report%20%221%22.txt") {
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
