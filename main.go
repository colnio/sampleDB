package main

import (
	"context"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sampleDB/internal/agents"
	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/dbschema"
	"sampleDB/internal/web"
)

// theApp holds all application-wide dependencies (DB pool, auth manager,
// template renderer, logger, timezone, and config). Initialized once in main
// and passed to all domain handler packages via their constructors.
var theApp *app.App

// initLocation loads the local timezone and returns it.
func initLocation() *time.Location {
	loc, err := time.LoadLocation("Local")
	if err != nil {
		log.Fatalf("Failed to load local timezone: %v", err)
	}
	return loc
}



func securityHeaders(next http.Handler, enableHSTS bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer-when-downgrade")
		if enableHSTS {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func redirectToHTTPS(publicHost, tlsPort string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := publicHost
		if host == "" {
			host = r.Host
		}
		if tlsPort != "" && tlsPort != "443" {
			if strings.Contains(host, ":") {
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = net.JoinHostPort(h, tlsPort)
				}
			} else {
				host = net.JoinHostPort(host, tlsPort)
			}
		} else if strings.Contains(host, ":") {
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
		}
		target := &url.URL{
			Scheme:   "https",
			Host:     host,
			Path:     r.URL.Path,
			RawQuery: r.URL.RawQuery,
		}
		http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
	})
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

	theApp, err = app.New(cfg, pool, authMgr, renderer, nil, nil, loc)
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

	handler := securityHeaders(mux, cfg.UseTLS)
	server := app.NewServer(cfg, handler)

	if cfg.UseTLS {
		if cfg.RedirectAddr != "" {
			go func() {
				redirectServer := &http.Server{
					Addr:         cfg.RedirectAddr,
					Handler:      redirectToHTTPS(cfg.PublicHost, cfg.TLSPort),
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

