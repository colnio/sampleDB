package main

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sampleDB/internal/agents"
	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/dbschema"
	"sampleDB/internal/web"
)

// initLocation loads the local timezone and returns it.
func initLocation() *time.Location {
	loc, err := time.LoadLocation("Local")
	if err != nil {
		log.Fatalf("Failed to load local timezone: %v", err)
	}
	return loc
}

func main() {
	loc := initLocation()
	cfg := app.LoadConfig()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	if err = dbschema.Ensure(ctx, pool); err != nil {
		log.Fatalf("Unable to ensure database schema: %v\n", err)
	}

	createdAdmin, err := app.SeedDefaultAdmin(ctx, pool, cfg)
	if err != nil {
		log.Fatalf("Unable to bootstrap default admin user: %v\n", err)
	}
	if createdAdmin {
		log.Printf("Created default admin user %q. Please change this password after first login.\n", cfg.AdminUsername)
	}

	authMgr := auth.NewManager(pool)
	if cfg.UseTLS {
		authMgr.SetCookieSecure(true)
	}
	authMgr.SetTemplateDir(cfg.TemplatesDir)

	renderer, err := web.NewRenderer(cfg.TemplatesDir, template.FuncMap{"markdown": web.RenderMarkdown}, loc)
	if err != nil {
		log.Fatalf("Unable to initialize template renderer: %v\n", err)
	}

	theApp, err := app.New(cfg, pool, authMgr, renderer, nil, nil, loc)
	if err != nil {
		log.Fatalf("Unable to initialize application: %v\n", err)
	}

	examplesDir := filepath.Join(cfg.StaticDir, "examples")
	agentsHdlrs, err := agents.New(cfg.TemplatesDir, examplesDir)
	if err != nil {
		log.Fatalf("Unable to parse agents templates: %v", err)
	}
	theApp.Agents = agentsHdlrs

	theApp.APINotFoundHandler = func(w http.ResponseWriter, r *http.Request) {
		web.Error(w, http.StatusNotFound, "not found")
	}

	mux := http.NewServeMux()
	theApp.RegisterRoutes(mux)

	// Ensure required directories exist
	if err = os.MkdirAll(cfg.UploadsDir, 0755); err != nil {
		log.Fatalf("Error creating uploads directory: %v\n", err)
	}

	if err = os.MkdirAll(filepath.Join(cfg.StaticDir, "css"), 0755); err != nil {
		log.Fatalf("Error creating static directories: %v\n", err)
	}

	handler := app.SecurityHeaders(mux, cfg.UseTLS)
	server := app.NewServer(cfg, handler)

	if cfg.UseTLS {
		if cfg.RedirectAddr != "" {
			go func() {
				redirectServer := &http.Server{
					Addr:         cfg.RedirectAddr,
					Handler:      app.RedirectToHTTPS(cfg.PublicHost, cfg.TLSPort),
					ReadTimeout:  5 * time.Second,
					WriteTimeout: 5 * time.Second,
					IdleTimeout:  10 * time.Second,
				}
				log.Printf("HTTP redirect server listening on %s", cfg.RedirectAddr)
				if err := redirectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Printf("HTTP redirect server error: %v", err)
				}
			}()
		}

		log.Printf("HTTPS server listening on %s", cfg.Addr)
		log.Fatal(server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile))
		return
	}

	log.Printf("HTTP server listening on %s", cfg.Addr)
	log.Fatal(server.ListenAndServe())
}

