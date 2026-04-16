package app

import (
	"net/http"

	"sampleDB/internal/auth"
	"sampleDB/internal/booking"
	"sampleDB/internal/samples"
	"sampleDB/internal/wiki"
)

// Handlers groups all HTTP handler functions the application needs.
// Sample-domain, wiki-domain, and booking-domain handlers are wired directly in
// RegisterRoutes via their respective Handlers types; they are no longer kept here.
type Handlers struct {
	// Admin
	AdminPage       http.HandlerFunc
	UpdateAccess    http.HandlerFunc
	SetAdmin        http.HandlerFunc
	ResetPassword   http.HandlerFunc
	AddEquipment    http.HandlerFunc
	DeleteEquipment http.HandlerFunc
	AddGroup        http.HandlerFunc
	DeleteGroup     http.HandlerFunc
	EquipmentReport http.HandlerFunc
	DeleteUser      http.HandlerFunc

	// Account / profile
	ChangePassword     http.HandlerFunc
	ProfileTokens      http.HandlerFunc
	ProfileTokenRevoke http.HandlerFunc

	// REST API v1 — users / equipment (samples API in samples.Handlers, wiki in wiki.Handlers)
	APIUsers     http.HandlerFunc
	APIEquipment http.HandlerFunc
	APINotFound  http.HandlerFunc

	// Public (no auth required) — non-sample pages
	AIAgents        http.HandlerFunc
	AIAgentExamples http.HandlerFunc
}

// RequireAdmin is a middleware that rejects non-admin users with a redirect to
// "/".  It must be applied after RequireAuth so that a valid session is already
// in the request context.
func (a *App) RequireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := auth.MustSessionFromContext(r.Context())

		var isAdmin bool
		err := a.DB.QueryRow(r.Context(),
			"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
			session.UserID).Scan(&isAdmin)

		if err != nil || !isAdmin {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

// RegisterRoutes wires all application HTTP routes onto mux.
// Sample-domain routes are registered using a freshly constructed
// samples.Handlers backed by the application's DB and Renderer.
func (a *App) RegisterRoutes(mux *http.ServeMux) {
	withAuth := a.Auth.RequireAuth
	adminH := a.RequireAdmin
	h := a.Handlers

	// Static files
	if a.Config.StaticDir != "" {
		fs := http.FileServer(http.Dir(a.Config.StaticDir))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
	}

	// Auth routes (no RequireAuth wrapper)
	mux.HandleFunc("/login", a.Auth.LoginHandler())
	mux.HandleFunc("/register", a.Auth.RegisterHandler())
	mux.HandleFunc("/logout", withAuth(a.Auth.LogoutHandler()))

	// --- Sample domain routes (wired via samples.Handlers) ---
	sampleSvc := samples.NewService(a.DB, nil)
	samplesHdlrs := samples.NewHandlers(sampleSvc, a.Renderer, a.Auth, samples.HandlerConfig{
		UploadsDir: a.Config.UploadsDir,
		BaseDir:    a.Config.BaseDir,
		PublicHost: a.Config.PublicHost,
		UseTLS:     a.Config.UseTLS,
	})
	mux.HandleFunc("/", withAuth(samplesHdlrs.MainPage))
	mux.HandleFunc("/samples/new", withAuth(samplesHdlrs.NewSample))
	mux.HandleFunc("/samples/edit/", withAuth(samplesHdlrs.EditSample))
	mux.HandleFunc("/samples/", withAuth(samplesHdlrs.HandleSample))
	mux.HandleFunc("/attachment/", withAuth(samplesHdlrs.HandleAttachment))
	mux.HandleFunc("/steps/", withAuth(samplesHdlrs.StepAction))
	mux.HandleFunc("/api/v1/samples", withAuth(samplesHdlrs.APISamples))
	mux.HandleFunc("/api/v1/samples/", withAuth(samplesHdlrs.APISamples))
	mux.HandleFunc("/api/v1/steps/", withAuth(samplesHdlrs.APISteps))
	mux.HandleFunc("/s/", samplesHdlrs.SharedSample)

	// --- Booking domain routes (wired via booking.Handlers) ---
	bookingSvc := booking.NewService(a.DB, a.Location)
	bookingHdlrs := booking.NewHandlers(bookingSvc, a.Renderer, a.Auth, a.Location)
	mux.HandleFunc("/booking", withAuth(bookingHdlrs.HandleBooking))
	mux.HandleFunc("/api/bookings", withAuth(bookingHdlrs.GetBookings))
	mux.HandleFunc("/booking/delete", withAuth(bookingHdlrs.DeleteBooking))

	// --- Wiki domain routes (wired via wiki.Handlers) ---
	wikiSvc := wiki.NewService(a.DB)
	wikiHdlrs := wiki.NewHandlers(wikiSvc, a.Renderer, wiki.HandlerConfig{
		UploadsDir: a.Config.UploadsDir,
		BaseDir:    a.Config.BaseDir,
	})
	mux.HandleFunc("/wiki", withAuth(wikiHdlrs.HandleWiki))
	mux.HandleFunc("/wiki/", withAuth(wikiHdlrs.HandleWiki))
	mux.HandleFunc("/wiki/attachment/", withAuth(wikiHdlrs.HandleAttachment))

	// REST API v1 routes — wiki
	mux.HandleFunc("/api/v1/wiki", withAuth(wikiHdlrs.APIWiki))
	mux.HandleFunc("/api/v1/wiki/", withAuth(wikiHdlrs.APIWiki))

	// Admin routes
	mux.HandleFunc("/admin", withAuth(adminH(h.AdminPage)))
	mux.HandleFunc("/admin/update-access", withAuth(adminH(h.UpdateAccess)))
	mux.HandleFunc("/admin/set-admin", withAuth(adminH(h.SetAdmin)))
	mux.HandleFunc("/admin/reset-password", withAuth(adminH(h.ResetPassword)))
	mux.HandleFunc("/admin/add-equipment", withAuth(adminH(h.AddEquipment)))
	mux.HandleFunc("/admin/delete-equipment/", withAuth(adminH(h.DeleteEquipment)))
	mux.HandleFunc("/admin/add-group", withAuth(adminH(h.AddGroup)))
	mux.HandleFunc("/admin/delete-group/", withAuth(adminH(h.DeleteGroup)))
	mux.HandleFunc("/admin/equipment-report", withAuth(adminH(h.EquipmentReport)))
	mux.HandleFunc("/admin/delete-user", withAuth(adminH(h.DeleteUser)))

	// Account / profile routes
	mux.HandleFunc("/change-password", withAuth(h.ChangePassword))
	mux.HandleFunc("/profile/tokens", withAuth(h.ProfileTokens))
	mux.HandleFunc("/profile/tokens/", withAuth(h.ProfileTokenRevoke))

	// REST API v1 routes — users / equipment
	mux.HandleFunc("/api/v1/users", withAuth(h.APIUsers))
	mux.HandleFunc("/api/v1/equipment", withAuth(h.APIEquipment))
	mux.HandleFunc("/api/", withAuth(h.APINotFound))

	// Public routes (no auth required)
	mux.HandleFunc("/agents", h.AIAgents)
	mux.HandleFunc("/agents/examples", h.AIAgentExamples)
	mux.HandleFunc("/agents/examples/", h.AIAgentExamples)
}
