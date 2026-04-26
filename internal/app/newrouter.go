package app

import (
	"html/template"
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

	loc, err := LoadLocation(cfg.Timezone)
	if err != nil || loc == nil {
		loc = time.UTC
	}

	// Try to load real templates so HTML pages render in integration tests
	// and alternative entry points. Falls back to the empty Renderer if the
	// template directory is unreachable — API-only callers are unaffected.
	renderer, rerr := web.NewRenderer(cfg.TemplatesDir, template.FuncMap{"markdown": web.RenderMarkdown}, loc)
	if rerr != nil || renderer == nil {
		renderer = &web.Renderer{}
	}
	theApp, _ := New(cfg, db, authMgr, renderer, nil, nil, loc)

	// agents.Handlers nil receiver returns 503 safely.
	if agentsHdlrs, err := agents.New(cfg.TemplatesDir, filepath.Join(cfg.StaticDir, "examples")); err == nil {
		theApp.Agents = agentsHdlrs
	}

	theApp.APINotFoundHandler = func(w http.ResponseWriter, r *http.Request) {
		web.Error(w, http.StatusNotFound, "not found")
	}

	mux := http.NewServeMux()
	theApp.RegisterRoutes(mux)
	return SecurityHeaders(mux, cfg.UseTLS)
}
