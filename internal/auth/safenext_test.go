package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeNext(t *testing.T) {
	tests := []struct {
		next, want string
	}{
		{"", "/dashboard"},
		{"/", "/"},
		{"/booking?date=2026-09-28", "/booking?date=2026-09-28"},
		{"/samples/12", "/samples/12"},
		{"//evil.com", "/dashboard"},
		{`/\evil.com`, "/dashboard"},
		{`/safe/../\attacker.example`, "/dashboard"},
		{`/./\evil`, "/dashboard"},
		{"/a/..//evil.com", "/a/..//evil.com"}, // http.Redirect cleans this to "/evil.com": local
		{"https://evil.com/", "/dashboard"},
		{"javascript:alert(1)", "/dashboard"},
		{"booking", "/dashboard"},
	}
	for _, tt := range tests {
		if got := SafeNext(tt.next, "/dashboard"); got != tt.want {
			t.Errorf("SafeNext(%q) = %q, want %q", tt.next, got, tt.want)
		}
	}
}

// TestSafeNext_FinalLocationIsLocal checks the Location header http.Redirect
// actually sends, since it path-cleans the target after SafeNext approved it.
func TestSafeNext_FinalLocationIsLocal(t *testing.T) {
	for _, next := range []string{
		`/safe/../\attacker.example`, "/a/..//evil.com", "/x/../../evil.com", "/%2F%2Fevil.com", "/booking?date=1",
	} {
		rr := httptest.NewRecorder()
		http.Redirect(rr, httptest.NewRequest(http.MethodPost, "/login", nil), SafeNext(next, "/dashboard"), http.StatusSeeOther)
		loc := rr.Header().Get("Location")
		if strings.HasPrefix(loc, "//") || strings.Contains(loc, `\`) || !strings.HasPrefix(loc, "/") {
			t.Errorf("next %q produced non-local Location %q", next, loc)
		}
	}
}
