package app

import (
	"net/http"
	"path/filepath"
	"time"

	"sampleDB/internal/agents"
	"sampleDB/internal/auth"
	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// NewRouter builds the full application HTTP handler from a config and DB pool.
// It is suitable for use by integration tests and alternative entry points.
// Template-dependent pages (agents, HTML views) degrade gracefully to 503
// when templates are unavailable.
func NewRouter(cfg Config, db dbiface.Pool) http.Handler {
	authMgr := auth.NewManager(db)
	authMgr.SetCookieSecure(cfg.UseTLS)
	authMgr.SetTemplateDir(cfg.TemplatesDir)

	loc, err := time.LoadLocation("Local")
	if err != nil || loc == nil {
		loc = time.UTC
	}

	// web.Renderer zero value is valid — no templates loaded means HTML pages
	// will fail, but API-only callers are unaffected.
	theApp, _ := New(cfg, db, authMgr, &web.Renderer{}, nil, nil, loc)

	// agents.Handlers nil receiver returns 503 safely.
	if agentsHdlrs, err := agents.New(cfg.TemplatesDir, filepath.Join(cfg.StaticDir, "examples")); err == nil {
		theApp.Agents = agentsHdlrs
	}

	theApp.APINotFoundHandler = func(w http.ResponseWriter, r *http.Request) {
		web.Error(w, http.StatusNotFound, "not found")
	}

	mux := http.NewServeMux()
	theApp.RegisterRoutes(mux)
	return mux
}
