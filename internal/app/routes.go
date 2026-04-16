package app

import (
	"net/http"

	"sampleDB/internal/auth"
)

// Handlers groups all HTTP handler functions the application needs.
// In future phases these will be replaced by domain-package methods; for now
// they are plain http.HandlerFunc values supplied by package main.
type Handlers struct {
	// Sample / main
	Main             http.HandlerFunc
	NewSample        http.HandlerFunc
	EditSample       http.HandlerFunc
	HandleSample     http.HandlerFunc
	HandleAttachment http.HandlerFunc

	// Booking
	Booking       http.HandlerFunc
	GetBookings   http.HandlerFunc
	DeleteBooking http.HandlerFunc

	// Wiki
	Wiki           http.HandlerFunc
	WikiAttachment http.HandlerFunc

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

	// Fabrication steps
	StepAction http.HandlerFunc

	// Account / profile
	ChangePassword     http.HandlerFunc
	ProfileTokens      http.HandlerFunc
	ProfileTokenRevoke http.HandlerFunc

	// REST API v1
	APISamples  http.HandlerFunc
	APISteps    http.HandlerFunc
	APIWiki     http.HandlerFunc
	APIUsers    http.HandlerFunc
	APIEquipment http.HandlerFunc
	APINotFound http.HandlerFunc

	// Public (no auth required)
	SharedSample    http.HandlerFunc
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

// RegisterRoutes wires all application HTTP routes onto mux using the handlers
// stored in a.Handlers.  Static assets under /static/ are served from
// a.Config.StaticDir when that field is non-empty.
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

	// Sample routes
	mux.HandleFunc("/", withAuth(h.Main))
	mux.HandleFunc("/samples/new", withAuth(h.NewSample))
	mux.HandleFunc("/samples/edit/", withAuth(h.EditSample))
	mux.HandleFunc("/samples/", withAuth(h.HandleSample))
	mux.HandleFunc("/attachment/", withAuth(h.HandleAttachment))

	// Booking routes
	mux.HandleFunc("/booking", withAuth(h.Booking))
	mux.HandleFunc("/api/bookings", withAuth(h.GetBookings))
	mux.HandleFunc("/booking/delete", withAuth(h.DeleteBooking))

	// Wiki routes
	mux.HandleFunc("/wiki", withAuth(h.Wiki))
	mux.HandleFunc("/wiki/", withAuth(h.Wiki))
	mux.HandleFunc("/wiki/attachment/", withAuth(h.WikiAttachment))

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

	// Fabrication step routes
	mux.HandleFunc("/steps/", withAuth(h.StepAction))

	// Account / profile routes
	mux.HandleFunc("/change-password", withAuth(h.ChangePassword))
	mux.HandleFunc("/profile/tokens", withAuth(h.ProfileTokens))
	mux.HandleFunc("/profile/tokens/", withAuth(h.ProfileTokenRevoke))

	// REST API v1 routes
	mux.HandleFunc("/api/v1/samples", withAuth(h.APISamples))
	mux.HandleFunc("/api/v1/samples/", withAuth(h.APISamples))
	mux.HandleFunc("/api/v1/steps/", withAuth(h.APISteps))
	mux.HandleFunc("/api/v1/wiki", withAuth(h.APIWiki))
	mux.HandleFunc("/api/v1/wiki/", withAuth(h.APIWiki))
	mux.HandleFunc("/api/v1/users", withAuth(h.APIUsers))
	mux.HandleFunc("/api/v1/equipment", withAuth(h.APIEquipment))
	mux.HandleFunc("/api/", withAuth(h.APINotFound))

	// Public routes (no auth required)
	mux.HandleFunc("/s/", h.SharedSample)
	mux.HandleFunc("/agents", h.AIAgents)
	mux.HandleFunc("/agents/examples", h.AIAgentExamples)
	mux.HandleFunc("/agents/examples/", h.AIAgentExamples)
}
