// Package app defines the central application struct and wires all
// domain-specific handlers (samples, wiki, booking, admin, profile) together
// into a single HTTP mux via RegisterRoutes.
package app

import (
	"errors"
	"log/slog"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// App aggregates every application-wide dependency so handlers no longer need
// to reach for package-level globals.
type App struct {
	Config   Config
	DB       dbiface.Pool
	Auth     *auth.Manager
	Renderer *web.Renderer
	Logger   *slog.Logger
	Clock    func() time.Time
	Location *time.Location
	// Handlers holds the HTTP handler functions registered by RegisterRoutes.
	// Populate this field before calling RegisterRoutes.
	Handlers Handlers
}

// New constructs an App, validating that the required fields are provided.
// DB, Auth, and Location are mandatory; the remaining fields get sensible
// defaults when omitted.
func New(
	cfg Config,
	db dbiface.Pool,
	authMgr *auth.Manager,
	renderer *web.Renderer,
	logger *slog.Logger,
	clock func() time.Time,
	loc *time.Location,
) (*App, error) {
	if db == nil {
		return nil, errors.New("app.New: DB must not be nil")
	}
	if authMgr == nil {
		return nil, errors.New("app.New: Auth must not be nil")
	}
	if loc == nil {
		return nil, errors.New("app.New: Location must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if clock == nil {
		clock = time.Now
	}
	if renderer == nil {
		renderer = &web.Renderer{}
	}
	return &App{
		Config:   cfg,
		DB:       db,
		Auth:     authMgr,
		Renderer: renderer,
		Logger:   logger,
		Clock:    clock,
		Location: loc,
	}, nil
}
