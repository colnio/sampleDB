package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"sampleDB/internal/admin"
	"sampleDB/internal/auth"
	"sampleDB/internal/booking"
	"sampleDB/internal/dashboard"
	"sampleDB/internal/profile"
	"sampleDB/internal/samples"
	"sampleDB/internal/wiki"
)

// noCacheStatic wraps a static-file handler so every response carries a
// Cache-Control header that disables raw browser caching but still allows
// conditional requests (ETag / If-Modified-Since). This means CSS and JS
// changes take effect on the next navigation without users having to hard
// refresh, at the cost of one conditional HTTP round-trip per asset per
// page load.
func noCacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is a middleware that rejects non-admin users with a redirect to
// "/".  It must be applied after RequireAuth so that a valid session is already
// in the request context.
func (a *App) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := auth.MustSessionFromContext(r.Context())

		var isAdmin bool
		err := a.DB.QueryRow(r.Context(),
			"SELECT COALESCE(admin, false) FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
			session.UserID).Scan(&isAdmin)

		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !isAdmin {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

// emailSetupPath is the page where a user without an email must add one.
const emailSetupPath = "/profile/email"

// RequireEmail is a middleware that sends browser-session users who have not
// linked an email yet to emailSetupPath until they do. It must be applied after
// RequireAuth. Bearer-token API sessions (empty CSRFToken), /api/ requests, the
// email page itself, and /logout are exempt so that scripts keep working and
// the user can always complete the form or sign out.
func (a *App) RequireEmail(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := auth.MustSessionFromContext(r.Context())
		path := r.URL.Path
		if session.CSRFToken == "" || path == emailSetupPath || path == "/logout" ||
			strings.HasPrefix(path, "/api/") {
			next(w, r)
			return
		}

		var hasEmail bool
		err := a.DB.QueryRow(r.Context(),
			"SELECT email IS NOT NULL FROM users WHERE user_id = $1",
			session.UserID).Scan(&hasEmail)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("app: RequireEmail lookup for user %d: %v", session.UserID, err)
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if hasEmail {
			next(w, r)
			return
		}

		target := emailSetupPath
		if r.Method == http.MethodGet {
			target += "?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		// htmx follows a 303 inside its XHR and would swap the email form into
		// the current page; HX-Redirect makes it do a full navigation instead.
		if strings.EqualFold(r.Header.Get("HX-Request"), "true") {
			w.Header().Set("HX-Redirect", target)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	}
}

// RegisterRoutes wires all application HTTP routes onto mux.
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	// Every authenticated route also passes the mandatory-email gate.
	withAuth := func(h http.HandlerFunc) http.HandlerFunc {
		return a.Auth.RequireAuth(a.RequireEmail(h))
	}
	adminMW := a.RequireAdmin
	withCSRF := RequireCSRF

	// Static files — served with Cache-Control: no-cache so the browser always
	// revalidates. Go's http.FileServer already supports If-Modified-Since /
	// If-None-Match via ServeFile, so the revalidation is cheap (304 Not
	// Modified) while guaranteeing that CSS/JS updates reach users as soon as
	// they navigate, without requiring a hard refresh.
	if a.Config.StaticDir != "" {
		fs := http.FileServer(http.Dir(a.Config.StaticDir))
		mux.Handle("/static/", http.StripPrefix("/static/", noCacheStatic(fs)))
	}

	// Auth routes (no RequireAuth wrapper)
	mux.HandleFunc("/login", a.Auth.LoginHandler())
	mux.HandleFunc("/register", a.Auth.RegisterHandler())
	mux.HandleFunc("/logout", withAuth(withCSRF(a.Auth.LogoutHandler())))

	// --- Sample domain routes (wired via samples.Handlers) ---
	sampleSvc := samples.NewService(a.DB)
	samplesHdlrs := samples.NewHandlers(sampleSvc, a.Renderer, a.Auth, samples.HandlerConfig{
		UploadsDir: a.Config.UploadsDir,
		BaseDir:    a.Config.BaseDir,
		PublicHost: a.Config.PublicHost,
		UseTLS:     a.Config.UseTLS,
	})
	samplesHdlrs.Activity = a.Activity
	// "/" is a catch-all in net/http.ServeMux — restrict it to the literal
	// root so unknown paths return 404 instead of silently redirecting.
	mux.HandleFunc("/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}))
	mux.HandleFunc("/samples", withAuth(samplesHdlrs.MainPage))
	mux.HandleFunc("/samples/new", withAuth(withCSRF(samplesHdlrs.NewSample)))
	mux.HandleFunc("/samples/export.csv", withAuth(samplesHdlrs.ExportCSV))
	mux.HandleFunc("/samples/edit/", withAuth(withCSRF(samplesHdlrs.EditSample)))
	mux.HandleFunc("/samples/", withAuth(withCSRF(samplesHdlrs.HandleSample)))
	mux.HandleFunc("/attachment/", withAuth(withCSRF(samplesHdlrs.HandleAttachment)))
	mux.HandleFunc("/steps/", withAuth(withCSRF(samplesHdlrs.StepAction)))
	mux.HandleFunc("/api/v1/samples", withAuth(samplesHdlrs.APISamples))
	mux.HandleFunc("/api/v1/samples/", withAuth(samplesHdlrs.APISamples))
	mux.HandleFunc("/api/v1/steps/", withAuth(samplesHdlrs.APISteps))
	mux.HandleFunc("/api/v1/attachments/", withAuth(samplesHdlrs.APIAttachments))
	mux.HandleFunc("/s/", samplesHdlrs.SharedSample)

	// --- Dashboard landing page ---
	dashSvc := dashboard.NewService(a.DB)
	dashHdlrs := dashboard.NewHandlers(dashSvc, a.Renderer, sampleSvc, a.Activity, a.Location)
	if a.Clock != nil {
		dashHdlrs.Now = a.Clock
	}
	mux.HandleFunc("/dashboard", withAuth(dashHdlrs.DashboardPage))

	// --- Booking domain routes (wired via booking.Handlers) ---
	bookingSvc := booking.NewService(a.DB, a.Location)
	bookingHdlrs := booking.NewHandlers(bookingSvc, a.Renderer, a.Auth, a.Location)
	bookingHdlrs.Activity = a.Activity
	mux.HandleFunc("/booking", withAuth(withCSRF(bookingHdlrs.HandleBooking)))
	mux.HandleFunc("/api/bookings", withAuth(bookingHdlrs.GetBookings))
	mux.HandleFunc("/booking/delete", withAuth(withCSRF(bookingHdlrs.DeleteBooking)))

	// --- Wiki domain routes (wired via wiki.Handlers) ---
	wikiSvc := wiki.NewService(a.DB)
	wikiHdlrs := wiki.NewHandlers(wikiSvc, a.Renderer, wiki.HandlerConfig{
		UploadsDir: a.Config.UploadsDir,
		BaseDir:    a.Config.BaseDir,
	})
	wikiHdlrs.Activity = a.Activity
	mux.HandleFunc("/wiki", withAuth(withCSRF(wikiHdlrs.HandleWiki)))
	mux.HandleFunc("/wiki/", withAuth(withCSRF(wikiHdlrs.HandleWiki)))
	mux.HandleFunc("/wiki/attachment/", withAuth(withCSRF(wikiHdlrs.HandleAttachment)))
	mux.HandleFunc("/api/v1/wiki", withAuth(wikiHdlrs.APIWiki))
	mux.HandleFunc("/api/v1/wiki/", withAuth(wikiHdlrs.APIWiki))

	// --- Admin domain routes (wired via admin.Handlers) ---
	adminSvc := admin.NewService(a.DB)
	adminHdlrs := admin.NewHandlers(adminSvc, a.Renderer, a.Auth)
	mux.HandleFunc("/admin", withAuth(adminMW(withCSRF(adminHdlrs.AdminPage))))
	mux.HandleFunc("/admin/update-access", withAuth(adminMW(withCSRF(adminHdlrs.UpdateAccess))))
	mux.HandleFunc("/admin/set-admin", withAuth(adminMW(withCSRF(adminHdlrs.SetAdmin))))
	mux.HandleFunc("/admin/reset-password", withAuth(adminMW(withCSRF(adminHdlrs.ResetPassword))))
	mux.HandleFunc("/admin/add-equipment", withAuth(adminMW(withCSRF(adminHdlrs.AddEquipment))))
	mux.HandleFunc("/admin/delete-equipment/", withAuth(adminMW(withCSRF(adminHdlrs.DeleteEquipment))))
	mux.HandleFunc("/admin/equipment/", withAuth(adminMW(withCSRF(func(w http.ResponseWriter, r *http.Request) {
		// Route equipment sub-paths:
		//   /admin/equipment/{id}/status
		//   /admin/equipment/{id}/groups/add
		//   /admin/equipment/{id}/groups/remove
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/status"):
			adminHdlrs.SetEquipmentStatus(w, r)
		case strings.HasSuffix(path, "/groups/add"):
			adminHdlrs.AddEquipmentGroup(w, r)
		case strings.HasSuffix(path, "/groups/remove"):
			adminHdlrs.RemoveEquipmentGroup(w, r)
		default:
			http.NotFound(w, r)
		}
	}))))
	mux.HandleFunc("/admin/add-group", withAuth(adminMW(withCSRF(adminHdlrs.AddGroup))))
	mux.HandleFunc("/admin/delete-group/", withAuth(adminMW(withCSRF(adminHdlrs.DeleteGroup))))
	mux.HandleFunc("/admin/equipment-report", withAuth(adminMW(adminHdlrs.EquipmentReport)))
	mux.HandleFunc("/admin/delete-user", withAuth(adminMW(withCSRF(adminHdlrs.DeleteUser))))
	mux.HandleFunc("/admin/set-email", withAuth(adminMW(withCSRF(adminHdlrs.SetEmail))))
	mux.HandleFunc("/admin/notifications", withAuth(adminMW(withCSRF(adminHdlrs.CreateNotification))))
	mux.HandleFunc("/admin/notifications/", withAuth(adminMW(withCSRF(adminHdlrs.DeleteNotification))))

	// REST API v1 — users / equipment (any authenticated user, matching original contract)
	mux.HandleFunc("/api/v1/users", withAuth(adminHdlrs.APIUsers))
	mux.HandleFunc("/api/v1/equipment", withAuth(adminHdlrs.APIEquipment))
	mux.HandleFunc("/api/notifications", withAuth(adminHdlrs.ListNotifications))
	apiNotFound := a.APINotFoundHandler
	if apiNotFound == nil {
		apiNotFound = http.NotFound
	}
	mux.HandleFunc("/api/", withAuth(apiNotFound))

	// --- Profile domain routes (wired via profile.Handlers) ---
	profileSvc := profile.NewService(a.DB)
	profileHdlrs := profile.NewHandlers(profileSvc, a.Renderer, a.Auth)
	mux.HandleFunc("/change-password", withAuth(withCSRF(profileHdlrs.ChangePassword)))
	mux.HandleFunc(emailSetupPath, withAuth(withCSRF(profileHdlrs.Email)))
	mux.HandleFunc("/profile/tokens", withAuth(withCSRF(profileHdlrs.ProfileTokens)))
	mux.HandleFunc("/profile/tokens/", withAuth(withCSRF(profileHdlrs.ProfileTokenRevoke)))

	// Public routes (no auth required)
	mux.HandleFunc("/agents", a.Agents.AgentsPage)
	mux.HandleFunc("/agents/examples", a.Agents.ExamplesPage)
	mux.HandleFunc("/agents/examples/", a.Agents.ExamplesPage)
}
