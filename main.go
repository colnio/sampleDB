package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/dbschema"
	"sampleDB/internal/web"
)

// BasePageData holds session-derived fields shared by all page templates.
// Handlers in wiki.go, sharing.go, and other root-level files embed this
// struct when building template data.
type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

type AIAgentExamplesPageData struct {
	DataProcessingHTML      template.HTML
	ScriptingAutomationHTML template.HTML
	SoftwareDevelopmentHTML template.HTML
}

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


var markdownImagePattern = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

func rewriteRelativeMarkdownImages(md, baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	return markdownImagePattern.ReplaceAllStringFunc(md, func(match string) string {
		parts := markdownImagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}

		alt := parts[1]
		rawTarget := strings.TrimSpace(parts[2])
		if rawTarget == "" {
			return match
		}

		pathPart := rawTarget
		suffix := ""
		if split := strings.IndexAny(rawTarget, " \t"); split >= 0 {
			pathPart = rawTarget[:split]
			suffix = rawTarget[split:]
		}

		trimmedPath := strings.Trim(pathPart, "<>")
		if strings.HasPrefix(trimmedPath, "http://") ||
			strings.HasPrefix(trimmedPath, "https://") ||
			strings.HasPrefix(trimmedPath, "/") ||
			strings.HasPrefix(trimmedPath, "#") ||
			strings.HasPrefix(trimmedPath, "data:") {
			return match
		}

		trimmedPath = strings.TrimPrefix(trimmedPath, "./")
		trimmedPath = strings.TrimPrefix(trimmedPath, ".\\")
		qualified := fmt.Sprintf("%s/%s", base, trimmedPath)
		if strings.HasPrefix(pathPart, "<") && strings.HasSuffix(pathPart, ">") {
			qualified = "<" + qualified + ">"
		}

		return fmt.Sprintf("![%s](%s%s)", alt, qualified, suffix)
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

	renderer, err := web.NewRenderer(cfg.TemplatesDir, template.FuncMap{"markdown": renderMarkdown}, loc)
	if err != nil {
		log.Fatalf("Unable to initialize template renderer: %v\n", err)
	}

	theApp, err = app.New(cfg, pool, authMgr, renderer, nil, nil, loc)
	if err != nil {
		log.Fatalf("Unable to initialize application: %v\n", err)
	}

	// Pre-parse standalone AI agent templates at startup (cached, not per-request).
	aiAgentsTmpl, err := template.ParseFiles(filepath.Join(cfg.TemplatesDir, "ai_agents.html"))
	if err != nil {
		log.Fatalf("Unable to parse ai_agents template: %v", err)
	}
	aiExamplesTmpl, err := template.ParseFiles(filepath.Join(cfg.TemplatesDir, "ai_agent_examples.html"))
	if err != nil {
		log.Fatalf("Unable to parse ai_agent_examples template: %v", err)
	}
	examplesBase := cfg.BaseDir
	if examplesBase == "" {
		examplesBase = "."
	}
	examplesDir := filepath.Join(examplesBase, "static", "examples")

	theApp.APINotFoundHandler = apiNotFoundHandler
	theApp.AIAgentsHandler = func(w http.ResponseWriter, r *http.Request) {
		if err := aiAgentsTmpl.Execute(w, nil); err != nil {
			http.Error(w, "Error rendering page", http.StatusInternalServerError)
		}
	}
	theApp.AIAgentExamplesHandler = func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agents/examples" && r.URL.Path != "/agents/examples/" {
			http.Redirect(w, r, "/agents/examples", http.StatusTemporaryRedirect)
			return
		}
		dataProcessingMD, err := os.ReadFile(filepath.Join(examplesDir, "data_processing.md"))
		if err != nil {
			http.Error(w, "Error loading example content", http.StatusInternalServerError)
			return
		}
		scriptingMD, err := os.ReadFile(filepath.Join(examplesDir, "scrpiting_automation.md"))
		if err != nil {
			http.Error(w, "Error loading example content", http.StatusInternalServerError)
			return
		}
		softwareMD, err := os.ReadFile(filepath.Join(examplesDir, "software_development.md"))
		if err != nil {
			http.Error(w, "Error loading example content", http.StatusInternalServerError)
			return
		}
		pageData := AIAgentExamplesPageData{
			DataProcessingHTML:      renderMarkdown(rewriteRelativeMarkdownImages(string(dataProcessingMD), "/static/examples")),
			ScriptingAutomationHTML: renderMarkdown(rewriteRelativeMarkdownImages(string(scriptingMD), "/static/examples")),
			SoftwareDevelopmentHTML: renderMarkdown(rewriteRelativeMarkdownImages(string(softwareMD), "/static/examples")),
		}
		if err := aiExamplesTmpl.Execute(w, pageData); err != nil {
			http.Error(w, "Error rendering page", http.StatusInternalServerError)
		}
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

