package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/app"
	"sampleDB/internal/auth"
	"sampleDB/internal/dbiface"
	"sampleDB/internal/dbschema"
	"sampleDB/internal/web"
)

type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

type ChangePasswordPageData struct {
	BasePageData
	Error   string
	Success string
}

type AIAgentExamplesPageData struct {
	DataProcessingHTML      template.HTML
	ScriptingAutomationHTML template.HTML
	SoftwareDevelopmentHTML template.HTML
}

var nonFileChars = regexp.MustCompile(`[^a-zA-Z0-9._\-]+`)

// getBasePageData fetches the admin flag for sessionUserID and returns a
// BasePageData suitable for embedding in any page template.
func getBasePageData(ctx context.Context, session auth.Session) (BasePageData, error) {
	var isAdmin bool
	err := theApp.DB.QueryRow(ctx,
		"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		session.UserID).Scan(&isAdmin)
	if err != nil {
		return BasePageData{}, err
	}
	return BasePageData{
		Username: session.Username,
		UserID:   session.UserID,
		IsAdmin:  isAdmin,
	}, nil
}

// theApp holds all application-wide dependencies, replacing the former
// package-level variables dbPool, appConfig, authManagerInstance and loc.
var theApp *app.App

func loadConfig() app.Config {
	cfg := app.Config{
		Addr:         getEnv("APP_ADDR", ":8010"),
		RedirectAddr: os.Getenv("APP_HTTP_REDIRECT_ADDR"),
		DatabaseURL:  getEnv("DATABASE_URL", "postgres://app:app@localhost:5432/sampledb"),
		TLSCertFile:  os.Getenv("TLS_CERT_FILE"),
		TLSKeyFile:   os.Getenv("TLS_KEY_FILE"),
		PublicHost:   os.Getenv("PUBLIC_HOST"),
	}

	cfg.UseTLS = cfg.TLSCertFile != "" && cfg.TLSKeyFile != ""
	if cfg.UseTLS && cfg.Addr == ":8010" {
		cfg.Addr = ":8443"
	}

	cfg.BaseDir = determineBaseDir()
	cfg.TemplatesDir = getEnv("TEMPLATES_DIR", filepath.Join(cfg.BaseDir, "templates"))
	cfg.StaticDir = getEnv("STATIC_DIR", filepath.Join(cfg.BaseDir, "static"))
	cfg.UploadsDir = getEnv("UPLOADS_DIR", filepath.Join(cfg.BaseDir, "uploads"))

	cfg.TLSPort = extractPort(cfg.Addr)

	if cfg.PublicHost == "" {
		if cfg.UseTLS {
			if cfg.TLSPort != "" && cfg.TLSPort != "443" {
				cfg.PublicHost = fmt.Sprintf("localhost:%s", cfg.TLSPort)
			} else {
				cfg.PublicHost = "localhost"
			}
		} else if cfg.TLSPort != "" && cfg.TLSPort != "80" {
			cfg.PublicHost = fmt.Sprintf("localhost:%s", cfg.TLSPort)
		} else {
			cfg.PublicHost = "localhost"
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func determineBaseDir() string {
	if base := os.Getenv("APP_BASE_DIR"); base != "" {
		if abs, err := filepath.Abs(base); err == nil {
			return abs
		}
	}

	if exe, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(filepath.Dir(exe)); err == nil {
			return abs
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if abs, err := filepath.Abs(wd); err == nil {
			return abs
		}
	}

	return "."
}

func extractPort(addr string) string {
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":")
	}

	_, port, err := net.SplitHostPort(addr)
	if err == nil {
		return port
	}

	// net.SplitHostPort requires an address with a port. If the provided value
	// did not include a port, fall back to empty string so callers can decide.
	return ""
}

func isHTMXRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
}

// renderTemplateSection renders a named partial section of a cached template group.
// group is the registered group name (e.g. "sample_detail", "wiki_view").
func renderTemplateSection(w http.ResponseWriter, group, section string, data interface{}) error {
	return theApp.Renderer.RenderSection(w, group, section, data)
}

// initLocation loads the local timezone and returns it.
func initLocation() *time.Location {
	loc, err := time.LoadLocation("Local")
	if err != nil {
		log.Fatalf("Failed to load local timezone: %v", err)
	}
	return loc
}

func sanitizeFilename(name string) string {
	base := strings.TrimSpace(filepath.Base(name))
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	cleanStem := nonFileChars.ReplaceAllString(stem, "-")
	cleanStem = strings.Trim(cleanStem, "-_. ")
	if cleanStem == "" {
		cleanStem = "file"
	}

	cleanExt := strings.TrimLeft(ext, ".")
	cleanExt = nonFileChars.ReplaceAllString(cleanExt, "")
	if cleanExt != "" {
		cleanExt = "." + strings.ToLower(cleanExt)
	}

	return cleanStem + cleanExt
}

func generateUniqueFilename(originalName string) (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}

	clean := sanitizeFilename(originalName)
	return fmt.Sprintf("%s_%s", hex.EncodeToString(buffer), clean), nil
}

func setDownloadHeaders(w http.ResponseWriter, filename string) {
	if filename == "" {
		return
	}
	safeName := strings.ReplaceAll(filename, "\"", "")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s",
			safeName, url.PathEscape(filename)))
}

func resolveAppPath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	clean := filepath.Clean(path)
	if theApp == nil || theApp.Config.BaseDir == "" {
		return clean
	}
	joined := filepath.Join(theApp.Config.BaseDir, clean)
	// Reject paths that escape the base directory.
	base := theApp.Config.BaseDir + string(os.PathSeparator)
	if joined != theApp.Config.BaseDir && !strings.HasPrefix(joined, base) {
		return ""
	}
	return joined
}

func resolveTemplatePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	clean := strings.TrimPrefix(path, "templates/")
	clean = strings.TrimPrefix(clean, "templates\\")
	clean = strings.TrimPrefix(clean, string(os.PathSeparator))
	clean = filepath.Clean(clean)

	if theApp == nil || theApp.Config.TemplatesDir == "" {
		return filepath.Join("templates", clean)
	}
	return filepath.Join(theApp.Config.TemplatesDir, clean)
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

func handleChangePassword(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	baseData, err := getBasePageData(r.Context(), session)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Redirect(w, r, "/logout", http.StatusSeeOther)
			return
		}
		http.Error(w, "Unable to load account information", http.StatusInternalServerError)
		return
	}

	data := ChangePasswordPageData{
		BasePageData: baseData,
	}

	switch r.Method {
	case http.MethodGet:
		renderChangePasswordTemplate(w, data)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = "Invalid form submission"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		currentPassword := strings.TrimSpace(r.FormValue("current_password"))
		newPassword := strings.TrimSpace(r.FormValue("new_password"))
		confirmPassword := strings.TrimSpace(r.FormValue("confirm_password"))

		if currentPassword == "" || newPassword == "" || confirmPassword == "" {
			data.Error = "All fields are required"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		if len(newPassword) < 8 {
			data.Error = "New password must be at least 8 characters long"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		if newPassword != confirmPassword {
			data.Error = "New passwords do not match"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		var storedHash string
		err := theApp.DB.QueryRow(r.Context(),
			`SELECT password_hash
	             FROM users
	             WHERE user_id = $1
	               AND COALESCE(deleted, false) = false`,
			session.UserID).Scan(&storedHash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Redirect(w, r, "/logout", http.StatusSeeOther)
				return
			}
			data.Error = "Unable to verify current password"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(currentPassword)); err != nil {
			data.Error = "Current password is incorrect"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		if currentPassword == newPassword {
			data.Error = "New password must be different from the current password"
			w.WriteHeader(http.StatusBadRequest)
			renderChangePasswordTemplate(w, data)
			return
		}

		newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			data.Error = "Failed to update password"
			w.WriteHeader(http.StatusInternalServerError)
			renderChangePasswordTemplate(w, data)
			return
		}

		cmdTag, err := theApp.DB.Exec(r.Context(),
			`UPDATE users
             SET password_hash = $1
             WHERE user_id = $2
               AND COALESCE(deleted, false) = false`,
			string(newHash), session.UserID)
		if err != nil || cmdTag.RowsAffected() == 0 {
			data.Error = "Failed to update password"
			w.WriteHeader(http.StatusInternalServerError)
			renderChangePasswordTemplate(w, data)
			return
		}

		data.Success = "Password updated successfully"
		renderChangePasswordTemplate(w, data)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func renderChangePasswordTemplate(w http.ResponseWriter, data ChangePasswordPageData) {
	if err := theApp.Renderer.Render(w, "change_password", data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// ProfileTokensPageData holds data for the API token management page.
type ProfileTokensPageData struct {
	BasePageData
	Tokens     []auth.APIToken
	PlainToken string // populated once, right after generation
	Error      string
	Success    string
}

func handleProfileTokens(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())
	baseData, err := getBasePageData(r.Context(), session)
	if err != nil {
		http.Error(w, "Error loading profile", http.StatusInternalServerError)
		return
	}

	data := ProfileTokensPageData{BasePageData: baseData}

	switch r.Method {
	case http.MethodGet:
		tokens, err := theApp.Auth.ListAPITokens(r.Context(), session.UserID)
		if err != nil {
			data.Error = "Could not load tokens"
		} else {
			data.Tokens = tokens
		}
		renderProfileTokensTemplate(w, data)

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			data.Error = "Invalid form submission"
			w.WriteHeader(http.StatusBadRequest)
			renderProfileTokensTemplate(w, data)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			data.Error = "Token name is required"
			w.WriteHeader(http.StatusBadRequest)
			renderProfileTokensTemplate(w, data)
			return
		}
		plain, err := theApp.Auth.CreateAPIToken(r.Context(), session.UserID, name)
		if err != nil {
			data.Error = "Could not create token"
			w.WriteHeader(http.StatusInternalServerError)
			renderProfileTokensTemplate(w, data)
			return
		}
		data.PlainToken = plain
		data.Success = "Token generated. Copy it now — it will not be shown again."
		tokens, _ := theApp.Auth.ListAPITokens(r.Context(), session.UserID)
		data.Tokens = tokens
		renderProfileTokensTemplate(w, data)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProfileTokenRevoke handles POST /profile/tokens/{id}/revoke
func handleProfileTokenRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	// Path: /profile/tokens/{id}/revoke
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts: ["profile", "tokens", "{id}", "revoke"]
	if len(parts) != 4 || parts[3] != "revoke" {
		http.NotFound(w, r)
		return
	}
	tokenID, err := strconv.Atoi(parts[2])
	if err != nil || tokenID <= 0 {
		http.Error(w, "Invalid token ID", http.StatusBadRequest)
		return
	}

	if err := theApp.Auth.RevokeAPIToken(r.Context(), tokenID, session.UserID); err != nil {
		if errors.Is(err, auth.ErrTokenNotFound) {
			http.Error(w, "Token not found", http.StatusNotFound)
		} else {
			http.Error(w, "Could not revoke token", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/profile/tokens", http.StatusSeeOther)
}

func renderProfileTokensTemplate(w http.ResponseWriter, data ProfileTokensPageData) {
	if err := theApp.Renderer.Render(w, "profile_tokens", data); err != nil {
		http.Error(w, "Template execution error", http.StatusInternalServerError)
	}
}

// ensureDefaultAdmin seeds a single administrator account when the installation is brand new.
// It leaves existing data untouched once any user records exist.
func ensureDefaultAdmin(ctx context.Context, pool dbiface.Pool) (bool, error) {
	var userCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}

	if userCount > 0 {
		return false, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return false, fmt.Errorf("hash default admin password: %w", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO users (username, password_hash, is_approved, admin)
		VALUES ($1, $2, true, true)
		ON CONFLICT (username) DO NOTHING`,
		"admin", string(hash)); err != nil {
		return false, fmt.Errorf("insert default admin: %w", err)
	}

	return true, nil
}

func handleAIAgents(w http.ResponseWriter, r *http.Request) {
	tmplPath := resolveTemplatePath("templates/ai_agents.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

func handleAIAgentExamples(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/agents/examples" {
		http.Redirect(w, r, "/agents/examples", http.StatusTemporaryRedirect)
		return
	}

	dataProcessingPath := resolveAppPath("static/examples/data_processing.md")
	dataProcessingMD, err := os.ReadFile(dataProcessingPath)
	if err != nil {
		http.Error(w, "Error loading example content", http.StatusInternalServerError)
		return
	}
	scriptingAutomationPath := resolveAppPath("static/examples/scrpiting_automation.md")
	scriptingAutomationMD, err := os.ReadFile(scriptingAutomationPath)
	if err != nil {
		http.Error(w, "Error loading example content", http.StatusInternalServerError)
		return
	}
	softwareDevelopmentPath := resolveAppPath("static/examples/software_development.md")
	softwareDevelopmentMD, err := os.ReadFile(softwareDevelopmentPath)
	if err != nil {
		http.Error(w, "Error loading example content", http.StatusInternalServerError)
		return
	}

	tmplPath := resolveTemplatePath("templates/ai_agent_examples.html")
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	pageData := AIAgentExamplesPageData{
		DataProcessingHTML:      renderMarkdown(rewriteRelativeMarkdownImages(string(dataProcessingMD), "/static/examples")),
		ScriptingAutomationHTML: renderMarkdown(rewriteRelativeMarkdownImages(string(scriptingAutomationMD), "/static/examples")),
		SoftwareDevelopmentHTML: renderMarkdown(rewriteRelativeMarkdownImages(string(softwareDevelopmentMD), "/static/examples")),
	}

	if err := tmpl.Execute(w, pageData); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
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
	cfg := loadConfig()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	if err = dbschema.Ensure(ctx, pool); err != nil {
		log.Fatalf("Unable to ensure database schema: %v\n", err)
	}

	createdAdmin, err := ensureDefaultAdmin(ctx, pool)
	if err != nil {
		log.Fatalf("Unable to bootstrap default admin user: %v\n", err)
	}
	if createdAdmin {
		log.Println("Created default admin user 'admin' with password 'admin'. Please change this password after first login.")
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

	theApp.Handlers = app.Handlers{
		ChangePassword:     handleChangePassword,
		ProfileTokens:      handleProfileTokens,
		ProfileTokenRevoke: handleProfileTokenRevoke,
		APINotFound:        apiNotFoundHandler,
		AIAgents:           handleAIAgents,
		AIAgentExamples:    handleAIAgentExamples,
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
	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if cfg.UseTLS {
		server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

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


// saveUploadedFile saves the file to disk and returns the file path
func saveUploadedFile(file io.Reader, originalName string) (string, error) {
	// Create uploads directory if it doesn't exist
	uploadDir := theApp.Config.UploadsDir
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}

	// Generate unique filename
	filename, err := generateUniqueFilename(originalName)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(uploadDir, filename)

	// Create new file
	dst, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Copy file content
	if _, err = io.Copy(dst, file); err != nil {
		return "", err
	}

	if theApp.Config.BaseDir != "" {
		if rel, err := filepath.Rel(theApp.Config.BaseDir, fullPath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel), nil
		}
	}

	return fullPath, nil
}


// isImage checks if a file is an image based on its content type
func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

