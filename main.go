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
	"mime"
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

	"sampleDB/internal/auth"
	"sampleDB/internal/dbiface"
	"sampleDB/internal/dbschema"
)

type Attachment struct {
	ID           int
	SampleID     int
	Address      string
	UploadedAt   time.Time
	OriginalName string // Added to store original filename
	ContentType  string // Added to store file type
	IsImage      bool
}

type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

// Sample represents a sample record in the database
type Sample struct {
	ID              int
	Name            string
	Description     string
	Keywords        string
	Owner           string
	OwnerUserID     *int
	OwnerUsername   *string
	Sample_prep     string
	SamplePrepHTML  template.HTML
	Attachments     []Attachment
	CreatedAt       time.Time
	ShareToken      *string // nil when not shared
	ShareVisibility string  // 'private', 'public', 'authenticated'
	FirstImageID    *int    // ID of the first image attachment, nil if none
}

type User struct {
	ID         int
	Username   string
	IsApproved bool
	GroupName  string
	CreatedAt  time.Time
}

type MainPageData struct {
	BasePageData
	Samples  []Sample
	Query    string
	DateFrom string
	DateTo   string
}

type SampleDetailPageData struct {
	BasePageData
	Sample      Sample
	Flash       string
	Error       string
	IsPartial   bool
	EditingPrep bool
	Users       []User
	Steps       []FabricationStep
	Equipment   []Equipment
	EditingStep int    // step_id currently in inline-edit mode (0 = none)
	ShareURL    string // full URL for the share link, empty when not shared
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

// Initialize a global database connection pool
var dbPool dbiface.Pool

var loc *time.Location

var nonFileChars = regexp.MustCompile(`[^a-zA-Z0-9._\-]+`)

var appConfig AppConfig

var authManagerInstance *auth.Manager

type AppConfig struct {
	Addr         string
	RedirectAddr string
	DatabaseURL  string
	TLSCertFile  string
	TLSKeyFile   string
	PublicHost   string
	TLSPort      string
	BaseDir      string
	TemplatesDir string
	StaticDir    string
	UploadsDir   string
	UseTLS       bool
}

func loadConfig() AppConfig {
	cfg := AppConfig{
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

func renderTemplateSection(w http.ResponseWriter, templatePath, section string, data interface{}) error {
	tmpl, err := parseTemplates(templatePath)
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, section, data)
}

// Add this function to booking.go
func initLocation() {
	var err error
	loc, err = time.LoadLocation("Local")
	if err != nil {
		log.Fatalf("Failed to load local timezone: %v", err)
	}
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

func originalFilenameFromPath(path string) string {
	base := filepath.Base(path)
	if idx := strings.Index(base, "_"); idx >= 0 && idx+1 < len(base) {
		return base[idx+1:]
	}
	return base
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
	if appConfig.BaseDir == "" {
		return clean
	}
	joined := filepath.Join(appConfig.BaseDir, clean)
	// Reject paths that escape the base directory.
	base := appConfig.BaseDir + string(os.PathSeparator)
	if joined != appConfig.BaseDir && !strings.HasPrefix(joined, base) {
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

	return filepath.Join(appConfig.TemplatesDir, clean)
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

	baseData, err := getBasePageData(session)
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
		err := dbPool.QueryRow(context.Background(),
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

		cmdTag, err := dbPool.Exec(context.Background(),
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
	tmpl, err := parseTemplates("templates/change_password.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
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
	baseData, err := getBasePageData(session)
	if err != nil {
		http.Error(w, "Error loading profile", http.StatusInternalServerError)
		return
	}

	data := ProfileTokensPageData{BasePageData: baseData}

	switch r.Method {
	case http.MethodGet:
		tokens, err := authManagerInstance.ListAPITokens(r.Context(), session.UserID)
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
		plain, err := authManagerInstance.CreateAPIToken(r.Context(), session.UserID, name)
		if err != nil {
			data.Error = "Could not create token"
			w.WriteHeader(http.StatusInternalServerError)
			renderProfileTokensTemplate(w, data)
			return
		}
		data.PlainToken = plain
		data.Success = "Token generated. Copy it now — it will not be shown again."
		tokens, _ := authManagerInstance.ListAPITokens(r.Context(), session.UserID)
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

	if err := authManagerInstance.RevokeAPIToken(r.Context(), tokenID, session.UserID); err != nil {
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
	tmpl, err := parseTemplates("templates/profile_tokens.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
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
	initLocation()
	cfg := loadConfig()
	appConfig = cfg

	ctx := context.Background()
	var err error
	dbPool, err = pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()

	if err = dbschema.Ensure(ctx, dbPool); err != nil {
		log.Fatalf("Unable to ensure database schema: %v\n", err)
	}

	createdAdmin, err := ensureDefaultAdmin(ctx, dbPool)
	if err != nil {
		log.Fatalf("Unable to bootstrap default admin user: %v\n", err)
	}
	if createdAdmin {
		log.Println("Created default admin user 'admin' with password 'admin'. Please change this password after first login.")
	}

	authManagerInstance = auth.NewManager(dbPool)
	if cfg.UseTLS {
		authManagerInstance.SetCookieSecure(true)
	}
	authManagerInstance.SetTemplateDir(cfg.TemplatesDir)

	mux := http.NewServeMux()

	// Set up static file serving
	fs := http.FileServer(http.Dir(cfg.StaticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Auth routes
	mux.HandleFunc("/login", authManagerInstance.LoginHandler())
	mux.HandleFunc("/register", authManagerInstance.RegisterHandler())
	mux.HandleFunc("/logout", authManagerInstance.RequireAuth(authManagerInstance.LogoutHandler()))

	// Sample management routes
	withAuth := authManagerInstance.RequireAuth
	mux.HandleFunc("/", withAuth(mainPageHandler))
	mux.HandleFunc("/samples/new", withAuth(newSampleHandler))
	mux.HandleFunc("/samples/edit/", withAuth(editSampleHandler))
	mux.HandleFunc("/samples/prep/", withAuth(samplePrepHandler))
	mux.HandleFunc("/samples/", withAuth(handleSample))
	mux.HandleFunc("/attachment/", withAuth(handleAttachment))
	mux.HandleFunc("/booking", withAuth(handleBooking))
	mux.HandleFunc("/api/bookings", withAuth(handleGetBookings))
	mux.HandleFunc("/booking/delete", withAuth(handleDeleteBooking))

	// Wiki routes
	mux.HandleFunc("/wiki", withAuth(handleWiki))
	mux.HandleFunc("/wiki/", withAuth(handleWiki))                      // Handles all wiki subpaths
	mux.HandleFunc("/wiki/attachment/", withAuth(handleAttachmentWiki)) // Handles wiki attachments

	// Admin routes
	mux.HandleFunc("/admin", withAuth(requireAdmin(handleAdminPage)))
	mux.HandleFunc("/admin/update-access", withAuth(requireAdmin(handleUpdateAccess)))
	mux.HandleFunc("/admin/set-admin", withAuth(requireAdmin(handleSetAdmin)))
	mux.HandleFunc("/admin/reset-password", withAuth(requireAdmin(handleResetPassword)))
	mux.HandleFunc("/admin/add-equipment", withAuth(requireAdmin(handleAddEquipment)))
	mux.HandleFunc("/admin/delete-equipment/", withAuth(requireAdmin(handleDeleteEquipment))) // Note trailing slash
	mux.HandleFunc("/admin/add-group", withAuth(requireAdmin(handleAddGroup)))
	mux.HandleFunc("/admin/delete-group/", withAuth(requireAdmin(handleDeleteGroup)))
	mux.HandleFunc("/admin/equipment-report", withAuth(requireAdmin(handleEquipmentReport)))
	mux.HandleFunc("/admin/delete-user", withAuth(requireAdmin(handleDeleteUser)))

	// Fabrication step actions: /steps/{id}/edit and /steps/{id}/delete
	mux.HandleFunc("/steps/", withAuth(handleStepAction))

	// Account management
	mux.HandleFunc("/change-password", withAuth(handleChangePassword))
	mux.HandleFunc("/profile/tokens", withAuth(handleProfileTokens))
	mux.HandleFunc("/profile/tokens/", withAuth(handleProfileTokenRevoke))

	// REST API v1 routes (Bearer token or session cookie auth)
	mux.HandleFunc("/api/v1/samples", withAuth(apiSamplesHandler))
	mux.HandleFunc("/api/v1/samples/", withAuth(apiSamplesHandler))
	mux.HandleFunc("/api/v1/steps/", withAuth(apiStepsHandler))
	mux.HandleFunc("/api/v1/wiki", withAuth(apiWikiHandler))
	mux.HandleFunc("/api/v1/wiki/", withAuth(apiWikiHandler))
	mux.HandleFunc("/api/v1/users", withAuth(apiUsersHandler))
	mux.HandleFunc("/api/v1/equipment", withAuth(apiEquipmentHandler))
	// Catch-all for unknown /api/ paths — return JSON 404 instead of falling through to the HTML root handler.
	mux.HandleFunc("/api/", withAuth(apiNotFoundHandler))

	// Public shared sample view — no auth middleware; visibility checked in handler
	mux.HandleFunc("/s/", handleSharedSample)

	// Public pages (no auth required)
	mux.HandleFunc("/agents", handleAIAgents)
	mux.HandleFunc("/agents/examples", handleAIAgentExamples)
	mux.HandleFunc("/agents/examples/", handleAIAgentExamples)

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

// publicBaseURL returns the scheme+host prefix used to construct share links.
func publicBaseURL() string {
	if appConfig.UseTLS {
		return "https://" + appConfig.PublicHost
	}
	return "http://" + appConfig.PublicHost
}

func parseTemplates(files ...string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"markdown": renderMarkdown,
		"split":    strings.Split,
		"trim":     strings.TrimSpace,
		"ownerSelected": func(ownerUserID *int, userID int) bool {
			return ownerUserID != nil && *ownerUserID == userID
		},
		"formatDate": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("2006-01-02")
		},
		"add": func(a, b int) int { return a + b },
	}

	// Always include base and header templates
	baseTemplates := []string{"templates/base.html", "templates/header.html"}
	resolved := make([]string, 0, len(files)+len(baseTemplates))
	for _, f := range baseTemplates {
		resolved = append(resolved, resolveTemplatePath(f))
	}
	for _, f := range files {
		resolved = append(resolved, resolveTemplatePath(f))
	}

	return template.New("").Funcs(funcMap).ParseFiles(resolved...)
}

// mainPageHandler serves the main page and handles search functionality
func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")
	var samples []Sample
	var err error

	// Get user info from context
	session := auth.MustSessionFromContext(r.Context())

	if query != "" {
		samples, err = searchSamples(query, dateFrom, dateTo)
		if err != nil {
			http.Error(w, "Error retrieving search results", http.StatusInternalServerError)
			return
		}
	} else {
		samples, err = getAllSamples(dateFrom, dateTo)
		if err != nil {
			http.Error(w, "Error retrieving samples", http.StatusInternalServerError)
			return
		}
	}

	// Execute the query
	row := dbPool.QueryRow(context.Background(), `SELECT admin
	FROM users
	WHERE username = $1
	  AND COALESCE(deleted, false) = false`, session.Username)

	data := MainPageData{
		BasePageData: BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: false},
		Samples:      samples,
		Query:        query,
		DateFrom:     dateFrom,
		DateTo:       dateTo,
	}

	if err := row.Scan(&data.BasePageData.IsAdmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Redirect(w, r, "/logout", http.StatusSeeOther)
			return
		}
		log.Printf("main: unable to load admin flag for %s: %v", session.Username, err)
	}
	tmpl, err := parseTemplates("templates/main.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) && r.Header.Get("HX-Target") == "samples-panel" {
		if err := tmpl.ExecuteTemplate(w, "samples_panel", data); err != nil {
			http.Error(w, "Error rendering samples", http.StatusInternalServerError)
		}
		return
	}

	tmpl.ExecuteTemplate(w, "base", data)
}

// Update handleSample function to properly handle the upload path pattern
func handleSample(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(pathParts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Handle upload requests: /samples/{id}/upload
	if len(pathParts) == 2 && pathParts[1] == "upload" {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed for upload", http.StatusMethodNotAllowed)
			return
		}
		uploadAttachmentHandler(w, r)
		return
	}

	// Handle steps reorder: /samples/{id}/steps/reorder
	if len(pathParts) == 3 && pathParts[1] == "steps" && pathParts[2] == "reorder" {
		handleFabricationStepsReorder(w, r)
		return
	}

	// Handle steps list/add: /samples/{id}/steps
	if len(pathParts) == 2 && pathParts[1] == "steps" {
		handleFabricationStepsEndpoint(w, r)
		return
	}

	// Handle share revoke: /samples/{id}/share/revoke
	if len(pathParts) == 3 && pathParts[1] == "share" && pathParts[2] == "revoke" {
		handleSampleShareRevoke(w, r)
		return
	}

	// Handle share generate/update: /samples/{id}/share
	if len(pathParts) == 2 && pathParts[1] == "share" {
		handleSampleShare(w, r)
		return
	}

	// Handle sample detail view: /samples/{id}
	if len(pathParts) == 1 {
		sampleDetailHandler(w, r)
		return
	}

	http.Error(w, "Invalid URL", http.StatusBadRequest)
}

// Update uploadAttachmentHandler to correctly get the sample ID from the URL
func uploadAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	// Get sample ID from URL (/samples/{id}/upload)
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(pathParts) != 2 || pathParts[1] != "upload" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	sampleID := pathParts[0]

	// Verify the caller owns the sample (or is admin) before accepting files.
	sampleIDInt, convErr := strconv.Atoi(sampleID)
	if convErr != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}
	if ownerErr := requireSampleOwner(r, sampleIDInt, session.UserID); ownerErr != nil {
		if errors.Is(ownerErr, errSampleNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
		} else if errors.Is(ownerErr, errForbidden) {
			http.Error(w, "Forbidden", http.StatusForbidden)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Limit total request body to 100 MB before parsing the multipart form.
	const maxUploadBytes = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	// Parse multipart form — allow up to 32 MB in memory; rest goes to temp files
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Unable to read the upload form.")
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Please choose at least one file to upload.")
			return
		}
		http.Error(w, "No files provided", http.StatusBadRequest)
		return
	}

	var errMsgs []string
	uploaded := 0
	for _, fh := range fileHeaders {
		f, openErr := fh.Open()
		if openErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("could not open %s", fh.Filename))
			continue
		}
		fp, saveErr := saveUploadedFile(f, fh.Filename)
		f.Close()
		if saveErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("could not save %s", fh.Filename))
			continue
		}
		if dbErr := addAttachment(sampleID, fp); dbErr != nil {
			_ = os.Remove(resolveAppPath(fp))
			errMsgs = append(errMsgs, fmt.Sprintf("could not store %s", fh.Filename))
			continue
		}
		uploaded++
	}

	var flash, errMsg string
	if uploaded > 0 {
		flash = fmt.Sprintf("%d file(s) uploaded", uploaded)
	}
	if len(errMsgs) > 0 {
		errMsg = strings.Join(errMsgs, "; ")
	}

	if isHTMXRequest(r) {
		renderSampleAttachmentsSection(w, r, session, sampleID, flash, errMsg)
		return
	}

	// Redirect back to sample detail page
	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
}

// searchSamples queries the database for samples by name, owner, or keywords
func searchSamples(query string, dateFrom, dateTo string) ([]Sample, error) {
	trimmedQuery := strings.TrimSpace(query)

	rawKeywords := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == ',' || r == ';'
	})

	var keywords []string
	for _, keyword := range rawKeywords {
		cleaned := strings.TrimSpace(keyword)
		if cleaned == "" {
			continue
		}
		keywords = append(keywords, strings.ToLower(cleaned))
	}

	// Construct the SQL query with `ANY` and `string_to_array`
	var whereClauses []string
	var args []interface{}
	for i, keyword := range keywords {
		whereClauses = append(whereClauses, fmt.Sprintf("$%d = ANY(string_to_array(lower(s.sample_keywords), ','))", i+1))
		args = append(args, keyword)
	}

	// Also match the full query string in `sample_name`
	nameParamIndex := len(args) + 1
	args = append(args, "%"+trimmedQuery+"%")

	// Match owner name as well
	ownerParamIndex := len(args) + 1
	args = append(args, "%"+trimmedQuery+"%")

	// Build the OR search conditions
	searchConds := []string{
		fmt.Sprintf("s.sample_name ILIKE $%d", nameParamIndex),
		fmt.Sprintf("s.sample_owner ILIKE $%d", ownerParamIndex),
	}
	searchConds = append(searchConds, whereClauses...)

	queryText := fmt.Sprintf(`
        SELECT s.sample_id, s.sample_name, s.sample_description, s.sample_keywords,
               COALESCE(s.sample_owner, ''), s.owner_user_id, u.username, s.created_at,
               first_img.attachment_id
          FROM samples s
          LEFT JOIN users u ON s.owner_user_id = u.user_id
          LEFT JOIN LATERAL (
              SELECT attachment_id FROM attachments
               WHERE source_type = 'sample' AND source_id = s.sample_id
                 AND attachment_address ~* '\.(jpg|jpeg|png|gif|webp|svg)$'
               ORDER BY attachment_id ASC LIMIT 1
          ) first_img ON true
         WHERE (%s)`, strings.Join(searchConds, " OR "))

	// Add date filtering
	if dateFrom != "" {
		dateFromParamIndex := len(args) + 1
		args = append(args, dateFrom)
		queryText = fmt.Sprintf(`%s AND s.created_at >= $%d`, queryText, dateFromParamIndex)
	}
	if dateTo != "" {
		dateToParamIndex := len(args) + 1
		// Add one day to dateTo to include the entire end date
		args = append(args, dateTo+" 23:59:59")
		queryText = fmt.Sprintf(`%s AND s.created_at <= $%d`, queryText, dateToParamIndex)
	}

	// Execute the query
	rows, err := dbPool.Query(context.Background(), queryText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect the results
	var samples []Sample
	for rows.Next() {
		var sample Sample
		err := rows.Scan(&sample.ID, &sample.Name, &sample.Description, &sample.Keywords, &sample.Owner, &sample.OwnerUserID, &sample.OwnerUsername, &sample.CreatedAt, &sample.FirstImageID)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

// getAllSamples retrieves all samples when there's no search query
func getAllSamples(dateFrom, dateTo string) ([]Sample, error) {
	queryText := `SELECT s.sample_id, s.sample_name, s.sample_description, s.sample_keywords,
		       COALESCE(s.sample_owner, ''), s.owner_user_id, u.username, COALESCE(s.sample_prep, ''), s.created_at,
		       first_img.attachment_id
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		  LEFT JOIN LATERAL (
		      SELECT attachment_id FROM attachments
		       WHERE source_type = 'sample' AND source_id = s.sample_id
		         AND attachment_address ~* '\.(jpg|jpeg|png|gif|webp|svg)$'
		       ORDER BY attachment_id ASC LIMIT 1
		  ) first_img ON true`
	var args []interface{}
	var whereClauses []string

	// Add date filtering
	if dateFrom != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.created_at >= $%d", len(args)+1))
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.created_at <= $%d", len(args)+1))
		args = append(args, dateTo+" 23:59:59")
	}

	if len(whereClauses) > 0 {
		queryText = queryText + " WHERE " + strings.Join(whereClauses, " AND ")
	}

	queryText = queryText + " ORDER BY s.sample_name ASC"

	rows, err := dbPool.Query(context.Background(), queryText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []Sample
	for rows.Next() {
		var sample Sample
		err := rows.Scan(&sample.ID, &sample.Name, &sample.Description, &sample.Keywords, &sample.Owner, &sample.OwnerUserID, &sample.OwnerUsername, &sample.Sample_prep, &sample.CreatedAt, &sample.FirstImageID)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

func sampleDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	sampleID := strings.TrimPrefix(r.URL.Path, "/samples/")

	data, err := loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Sample not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error loading sample", http.StatusInternalServerError)
		}
		return
	}

	tmpl, err := parseTemplates(
		"templates/sample_detail.html",
		"templates/fabrication_steps.html",
		"templates/step_form.html",
	)
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "base", data)

}

func getSampleByID(sampleID string) (Sample, error) {
	var sample Sample
	err := dbPool.QueryRow(context.Background(),
		`SELECT s.sample_id, s.sample_name, COALESCE(s.sample_description, ''), COALESCE(s.sample_keywords, ''),
		        COALESCE(s.sample_owner, ''), s.owner_user_id, u.username,
		        COALESCE(s.sample_prep, ''), s.created_at,
		        s.share_token, COALESCE(s.share_visibility, 'private')
		   FROM samples s
		   LEFT JOIN users u ON s.owner_user_id = u.user_id
		  WHERE s.sample_id=$1`, sampleID).Scan(
		&sample.ID, &sample.Name, &sample.Description, &sample.Keywords, &sample.Owner,
		&sample.OwnerUserID, &sample.OwnerUsername,
		&sample.Sample_prep, &sample.CreatedAt,
		&sample.ShareToken, &sample.ShareVisibility)
	if err != nil {
		return sample, err
	}

	// Fetch attachments
	sample.Attachments, err = getAttachments(sampleID)
	if err != nil {
		return sample, err
	}

	sample.SamplePrepHTML = renderMarkdown(sample.Sample_prep)
	return sample, nil
}

// getApprovedUsers returns all approved, non-deleted users ordered by username.
func getApprovedUsers() ([]User, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT user_id, username FROM users
		  WHERE is_approved = true AND COALESCE(deleted, false) = false
		  ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func loadSampleDetailData(ctx context.Context, session auth.Session, sampleID string) (SampleDetailPageData, error) {
	sample, err := getSampleByID(sampleID)
	if err != nil {
		return SampleDetailPageData{}, err
	}

	users, err := getApprovedUsers()
	if err != nil {
		return SampleDetailPageData{}, err
	}

	equipment, err := getAllEquipment()
	if err != nil {
		return SampleDetailPageData{}, err
	}

	sampleIDInt, _ := strconv.Atoi(sampleID)
	steps, _ := getFabricationSteps(ctx, sampleIDInt) // non-fatal: show empty on error

	shareURL := ""
	if sample.ShareToken != nil && *sample.ShareToken != "" {
		shareURL = publicBaseURL() + "/s/" + *sample.ShareToken
	}

	data := SampleDetailPageData{
		BasePageData: BasePageData{Username: session.Username, UserID: session.UserID},
		Sample:       sample,
		Users:        users,
		Equipment:    equipment,
		Steps:        steps,
		ShareURL:     shareURL,
	}

	if err := dbPool.QueryRow(ctx,
		"SELECT admin FROM users WHERE user_id = $1",
		session.UserID,
	).Scan(&data.BasePageData.IsAdmin); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return SampleDetailPageData{}, err
		}
	}

	return data, nil
}

func renderSampleAttachmentsSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleID, flash, errMsg string) {
	data, err := loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true

	if err := renderTemplateSection(w, "templates/sample_detail.html", "sample_attachments", data); err != nil {
		http.Error(w, "Error rendering attachments", http.StatusInternalServerError)
	}
}

func renderSampleEditSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleID, flash, errMsg string) {
	data, err := loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true

	if err := renderTemplateSection(w, "templates/sample_detail.html", "sample_edit_form", data); err != nil {
		http.Error(w, "Error rendering form", http.StatusInternalServerError)
	}
}

func renderSamplePrepSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleID, flash, errMsg string, editing bool) {
	data, err := loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true
	data.EditingPrep = editing

	if err := renderTemplateSection(w, "templates/sample_detail.html", "sample_prep_panel", data); err != nil {
		http.Error(w, "Error rendering sample prep", http.StatusInternalServerError)
	}
}

// editSampleHandler updates a sample's details in the database
func editSampleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	// Extract sample ID from URL by removing "/samples/edit/"
	sampleID := strings.TrimPrefix(r.URL.Path, "/samples/edit/")

	sampleIDInt, err := strconv.Atoi(sampleID)
	if err != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}
	if err := requireSampleOwner(r, sampleIDInt, session.UserID); err != nil {
		if errors.Is(err, errSampleNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
		} else if errors.Is(err, errForbidden) {
			http.Error(w, "Forbidden", http.StatusForbidden)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Parse form data
	err = r.ParseForm()
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleEditSection(w, r, session, sampleID, "", "Unable to read the form submission.")
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	keywords := r.FormValue("keywords")
	hasSamplePrep := r.Form.Has("sample_prep")
	sample_prep := r.FormValue("sample_prep")

	ownerUserIDStr := r.FormValue("owner_user_id")
	var ownerUserID *int
	var ownerName string
	if ownerUserIDStr != "" {
		id, parseErr := strconv.Atoi(ownerUserIDStr)
		if parseErr == nil {
			ownerUserID = &id
			// Resolve username for legacy sample_owner text field
			_ = dbPool.QueryRow(context.Background(),
				"SELECT username FROM users WHERE user_id = $1", id).Scan(&ownerName)
		}
	}

	var (
		query string
		args  []interface{}
	)

	if hasSamplePrep {
		query = "UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3, sample_owner=$4, owner_user_id=$5, sample_prep=$6 WHERE sample_id=$7"
		args = []interface{}{name, description, keywords, ownerName, ownerUserID, sample_prep, sampleID}
	} else {
		query = "UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3, sample_owner=$4, owner_user_id=$5 WHERE sample_id=$6"
		args = []interface{}{name, description, keywords, ownerName, ownerUserID, sampleID}
	}

	_, err = dbPool.Exec(context.Background(), query, args...)
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleEditSection(w, r, session, sampleID, "", "Failed to update the sample.")
			return
		}
		log.Printf("editSampleHandler: update error: %v", err)
		http.Error(w, "Error updating sample", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		renderSampleEditSection(w, r, session, sampleID, "Changes saved", "")
		return
	}

	// Redirect to the sample's detail page to display updated info
	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
}

// newSampleHandler handles both displaying the form and processing the submission
func newSampleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		session := auth.MustSessionFromContext(r.Context())

		baseData, err := getBasePageData(session)
		if err != nil {
			http.Error(w, "Error loading user information", http.StatusInternalServerError)
			return
		}

		users, err := getApprovedUsers()
		if err != nil {
			http.Error(w, "Error loading user list", http.StatusInternalServerError)
			return
		}

		data := struct {
			BasePageData
			Users []User
		}{
			BasePageData: baseData,
			Users:        users,
		}

		tmpl, err := parseTemplates("templates/new_sample.html")
		if err != nil {
			http.Error(w, "Error loading template", http.StatusInternalServerError)
			return
		}

		tmpl.ExecuteTemplate(w, "base", data)

		return
	} else if r.Method == http.MethodPost {
		// Process the form submission
		name := r.FormValue("name")
		description := r.FormValue("description")
		keywords := r.FormValue("keywords")
		prep := r.FormValue("sample_prep")

		ownerUserIDStr := r.FormValue("owner_user_id")
		var ownerUserID *int
		var ownerName string
		if ownerUserIDStr != "" {
			id, err := strconv.Atoi(ownerUserIDStr)
			if err == nil {
				ownerUserID = &id
				// Resolve username for legacy sample_owner text field
				_ = dbPool.QueryRow(context.Background(),
					"SELECT username FROM users WHERE user_id = $1", id).Scan(&ownerName)
			}
		}

		if strings.TrimSpace(name) == "" {
			http.Error(w, "Sample name is required", http.StatusBadRequest)
			return
		}

		// Insert the new sample into the database
		_, err := dbPool.Exec(context.Background(),
			`INSERT INTO samples (sample_name, sample_description, sample_keywords, sample_prep, sample_owner, owner_user_id)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			name, description, keywords, prep, ownerName, ownerUserID)
		if err != nil {
			log.Printf("newSampleHandler: insert error: %v", err)
			http.Error(w, "Error adding sample", http.StatusInternalServerError)
			return
		}

		// Redirect to the main page to show the new sample
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		// Return 405 Method Not Allowed for other methods
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// saveUploadedFile saves the file to disk and returns the file path
func saveUploadedFile(file io.Reader, originalName string) (string, error) {
	// Create uploads directory if it doesn't exist
	uploadDir := appConfig.UploadsDir
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

	if appConfig.BaseDir != "" {
		if rel, err := filepath.Rel(appConfig.BaseDir, fullPath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel), nil
		}
	}

	return fullPath, nil
}

// getAttachments retrieves all attachments for a sample
func getAttachments(sampleID string) ([]Attachment, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT attachment_id, source_id, attachment_address, uploaded_at 
         FROM attachments 
         WHERE source_type = 'sample' AND source_id = $1`,
		sampleID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var att Attachment
		err := rows.Scan(&att.ID, &att.SampleID, &att.Address, &att.UploadedAt)
		if err != nil {
			return nil, err
		}
		att.OriginalName = originalFilenameFromPath(att.Address)
		att.ContentType = mime.TypeByExtension(filepath.Ext(att.OriginalName))
		att.IsImage = isImage(att.ContentType)
		attachments = append(attachments, att)
	}
	return attachments, rows.Err()
}

// addAttachment stores a new attachment in the database
func addAttachment(sampleID string, filepath string) error {
	_, err := dbPool.Exec(context.Background(),
		`INSERT INTO attachments (source_id, attachment_address, source_type) 
         VALUES ($1, $2, 'sample')`,
		sampleID, filepath)
	return err
}

// deleteAttachment removes an attachment from both database and filesystem
func deleteAttachment(attachmentID string) error {
	// First get the file path
	var filepath string
	err := dbPool.QueryRow(context.Background(),
		"SELECT attachment_address FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID,
	).Scan(&filepath)
	if err != nil {
		return err
	}

	// Delete from database
	_, err = dbPool.Exec(context.Background(),
		"DELETE FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID)
	if err != nil {
		return err
	}

	// Delete file from filesystem
	if removeErr := os.Remove(resolveAppPath(filepath)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return removeErr
	}
	return nil
}

// isImage checks if a file is an image based on its content type
func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}


func downloadAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get attachment ID from URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	attachmentID := parts[2]

	// Get file path from database
	var filePath string
	err := dbPool.QueryRow(context.Background(),
		"SELECT attachment_address FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID,
	).Scan(&filePath)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	original := originalFilenameFromPath(filePath)
	// Only force download for non-image files; images must be served inline so
	// the gallery <img> tags can display them in the browser.
	if !isImage(mime.TypeByExtension(filepath.Ext(filePath))) {
		setDownloadHeaders(w, original)
	}
	// Serve the file
	resolved := resolveAppPath(filePath)
	if resolved == "" {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, resolved)
}

func deleteAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	// Get attachment ID from URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	attachmentID := parts[2]

	// Get sample ID before deleting the attachment
	var sampleID string
	err := dbPool.QueryRow(context.Background(),
		"SELECT source_id FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID,
	).Scan(&sampleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Attachment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to remove attachment.", http.StatusInternalServerError)
		return
	}

	// Verify the caller owns the sample (or is an admin).
	sampleIDInt, convErr := strconv.Atoi(sampleID)
	if convErr != nil {
		http.Error(w, "Invalid sample ID", http.StatusInternalServerError)
		return
	}
	if ownerErr := requireSampleOwner(r, sampleIDInt, session.UserID); ownerErr != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "You do not have permission to delete this attachment.")
			return
		}
		if errors.Is(ownerErr, errSampleNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete the attachment
	err = deleteAttachment(attachmentID)
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Failed to remove attachment.")
			return
		}
		http.Error(w, "Error deleting attachment", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		renderSampleAttachmentsSection(w, r, session, sampleID, "Attachment removed", "")
		return
	}

	// Redirect back to the sample page
	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
}

func handleAttachment(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Check if this is a delete request
	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/delete") {
		// Remove "/delete" from the end to get the attachment ID
		// attachmentID := pathParts[2]
		deleteAttachmentHandler(w, r)
		return
	}

	// Handle download request
	if r.Method == http.MethodGet {
		downloadAttachmentHandler(w, r)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
