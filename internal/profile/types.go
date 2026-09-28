// Package profile provides the domain types, service, and HTTP handlers for
// user profile management (change-password, API token management).
package profile

import "sampleDB/internal/auth"

// BasePageData holds session-derived fields shared by all page templates.
type BasePageData struct {
	Username  string
	UserID    int
	IsAdmin   bool
	CSRFToken string
}

// ChangePasswordPageData is the template data for the change-password page.
type ChangePasswordPageData struct {
	BasePageData
	Error   string
	Success string
}

// EmailPageData is the template data for the /profile/email page.
type EmailPageData struct {
	BasePageData
	Email string
	// Next is where to continue after the first email is saved (already
	// validated by auth.SafeNext before redirecting).
	Next string
	// FirstTime is true while the user has no email linked; the page is then
	// the mandatory prompt shown right after login.
	FirstTime bool
	Error     string
	Success   string
}

// ProfileTokensPageData holds data for the API token management page.
type ProfileTokensPageData struct {
	BasePageData
	Tokens     []auth.APIToken
	PlainToken string // populated once, right after generation
	Error      string
	Success    string
}
