package profile_test

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/profile"
	"sampleDB/internal/web"
)

// renderTokensPage executes the profile_tokens template group against the
// real templates directory and returns the written response body for assertion.
func renderTokensPage(t *testing.T, data profile.ProfileTokensPageData) string {
	t.Helper()
	baseDir, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatalf("abs templates dir: %v", err)
	}
	r, err := web.NewRenderer(baseDir, nil, time.UTC)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	rr := httptest.NewRecorder()
	if err := r.Render(rr, "profile_tokens", data); err != nil {
		t.Fatalf("render: %v", err)
	}
	return rr.Body.String()
}

// TestProfileTokens_RendersRevokeButtonWithStandardClasses asserts that the
// revoke form uses the standardized button--destructive button--small classes
// (QA UX-1 — prevents regression to ad-hoc "delete-btn" styling).
func TestProfileTokens_RendersRevokeButtonWithStandardClasses(t *testing.T) {
	now := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	used := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	data := profile.ProfileTokensPageData{
		BasePageData: profile.BasePageData{Username: "testuser", UserID: 1},
		Tokens: []auth.APIToken{
			{TokenID: 10, UserID: 1, Name: "skill_check", CreatedAt: now, LastUsedAt: &used, Revoked: false},
		},
	}
	body := renderTokensPage(t, data)

	if !strings.Contains(body, `action="/profile/tokens/10/revoke"`) {
		t.Errorf("render missing revoke form action for token id 10")
	}
	if !strings.Contains(body, "button--destructive") {
		t.Errorf("revoke button missing button--destructive class")
	}
	if !strings.Contains(body, "button--small") {
		t.Errorf("revoke button missing button--small class")
	}
}

// TestProfileTokens_RendersStatusChipsWithDot asserts that active and revoked
// tokens render using status-chip dots so the design matches screenshot #9.
func TestProfileTokens_RendersStatusChipsWithDot(t *testing.T) {
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	data := profile.ProfileTokensPageData{
		BasePageData: profile.BasePageData{Username: "testuser", UserID: 1},
		Tokens: []auth.APIToken{
			{TokenID: 1, UserID: 1, Name: "active_token", CreatedAt: now, Revoked: false},
			{TokenID: 2, UserID: 1, Name: "revoked_token", CreatedAt: now, Revoked: true},
		},
	}
	body := renderTokensPage(t, data)

	wants := []string{
		"status-chip status-chip--active",
		"status-chip status-chip--archived",
		"status-chip__dot",
		"Active",
		"Revoked",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("tokens page render missing %q", w)
		}
	}
}

// TestProfileTokens_RendersTwoColumnLayoutAndUsageBlock asserts that the page
// uses the two-column (Generate | Existing) layout plus a Usage card with the
// Authorization bearer example (screenshot #9).
func TestProfileTokens_RendersTwoColumnLayoutAndUsageBlock(t *testing.T) {
	data := profile.ProfileTokensPageData{
		BasePageData: profile.BasePageData{Username: "testuser", UserID: 1},
	}
	body := renderTokensPage(t, data)

	wants := []string{
		"tokens-grid",
		"tokens-generate",
		"tokens-list",
		"tokens-usage",
		"Authorization: Bearer",
		"Generate new token",
		"Existing tokens",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("tokens page render missing %q", w)
		}
	}
}
