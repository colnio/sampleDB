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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/auth"
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
	ID             int
	Name           string
	Description    string
	Keywords       string
	Owner          string
	Sample_prep    string
	SamplePrepHTML template.HTML
	Attachments    []Attachment
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
	Samples []Sample
	Query   string
}

type SampleDetailPageData struct {
	BasePageData
	Sample      Sample
	Flash       string
	Error       string
	IsPartial   bool
	EditingPrep bool
}

type ChangePasswordPageData struct {
	BasePageData
	Error   string
	Success string
}

// Initialize a global database connection pool
var dbPool *pgxpool.Pool

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
		if cfg.UseTLS && cfg.TLSPort != "" && cfg.TLSPort != "443" {
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

func hxTargetIs(r *http.Request, target string) bool {
	return isHTMXRequest(r) && r.Header.Get("HX-Target") == target
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
	return filepath.Join(appConfig.BaseDir, clean)
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

func main() {
	initLocation()
	cfg := loadConfig()
	appConfig = cfg

	var err error
	dbPool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()

	if err = dbschema.Ensure(context.Background(), dbPool); err != nil {
		log.Fatalf("Unable to ensure database schema: %v\n", err)
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

	// Account management
	mux.HandleFunc("/change-password", withAuth(handleChangePassword))

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

func parseTemplates(files ...string) (*template.Template, error) {
	// Always include base and header templates
	baseTemplates := []string{"templates/base.html", "templates/header.html"}
	resolved := make([]string, 0, len(files)+len(baseTemplates))
	for _, f := range baseTemplates {
		resolved = append(resolved, resolveTemplatePath(f))
	}
	for _, f := range files {
		resolved = append(resolved, resolveTemplatePath(f))
	}

	return template.ParseFiles(resolved...)
}

// mainPageHandler serves the main page and handles search functionality
func mainPageHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	var samples []Sample
	var err error

	// Get user info from context
	session := auth.MustSessionFromContext(r.Context())

	if query != "" {
		samples, err = searchSamples(query)
		if err != nil {
			http.Error(w, "Error retrieving search results", http.StatusInternalServerError)
			return
		}
	} else {
		samples, err = getAllSamples()
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

	if hxTargetIs(r, "samples-panel") {
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

	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Unable to read the upload form.")
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Please choose a file to upload.")
			return
		}
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save file and get filepath
	filepath, err := saveUploadedFile(file, header.Filename)
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Unable to save the file. Try again.")
			return
		}
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	// Add to database
	err = addAttachment(sampleID, filepath)
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleAttachmentsSection(w, r, session, sampleID, "", "Could not store attachment metadata.")
			return
		}
		http.Error(w, "Error storing attachment info", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		renderSampleAttachmentsSection(w, r, session, sampleID, "Attachment uploaded", "")
		return
	}

	// Redirect back to sample detail page
	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
}

// searchSamples queries the database for samples by name or keywords
func searchSamples(query string) ([]Sample, error) {
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
		whereClauses = append(whereClauses, fmt.Sprintf("$%d = ANY(string_to_array(lower(sample_keywords), ','))", i+1))
		args = append(args, keyword)
	}

	// Also match the full query string in `sample_name`
	nameParamIndex := len(args) + 1
	args = append(args, "%"+trimmedQuery+"%")

	queryText := fmt.Sprintf(`
        SELECT sample_id, sample_name, sample_description, sample_keywords, sample_owner
        FROM samples
        WHERE sample_name ILIKE $%d`, nameParamIndex)
	if len(whereClauses) > 0 {
		queryText = fmt.Sprintf(`%s OR %s`, queryText, strings.Join(whereClauses, " OR "))
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
		err := rows.Scan(&sample.ID, &sample.Name, &sample.Description, &sample.Keywords, &sample.Owner)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}

	return samples, nil
}

// getAllSamples retrieves all samples when there’s no search query
func getAllSamples() ([]Sample, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT sample_id, sample_name, sample_description, sample_keywords, sample_owner, coalesce(sample_prep, '') FROM samples order by sample_name asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var samples []Sample
	for rows.Next() {
		var sample Sample
		err := rows.Scan(&sample.ID, &sample.Name, &sample.Description, &sample.Keywords, &sample.Owner, &sample.Sample_prep)
		if err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}

	return samples, nil
}

// getSamples retrieves all samples from the database
func getSamples() ([]Sample, error) {
	rows, err := dbPool.Query(context.Background(), "SELECT sample_id, sample_name, sample_description, sample_keywords, sample_owner FROM samples")
	if err != nil {
		fmt.Printf("%s\n", err)
		return nil, err
	}
	defer rows.Close()

	var samples []Sample
	for rows.Next() {
		// fmt.Printf("parsing rows\n")
		var s Sample
		err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Keywords, &s.Owner)
		if err != nil {
			return nil, err
		}
		samples = append(samples, s)
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

	tmpl, err := parseTemplates("templates/sample_detail.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "base", data)

}

func getSampleByID(sampleID string) (Sample, error) {
	var sample Sample
	err := dbPool.QueryRow(context.Background(),
		`SELECT sample_id, sample_name, sample_description, sample_keywords, sample_owner, coalesce(sample_prep, '') 
         FROM samples WHERE sample_id=$1`, sampleID).Scan(
		&sample.ID, &sample.Name, &sample.Description, &sample.Keywords, &sample.Owner, &sample.Sample_prep)
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

func loadSampleDetailData(ctx context.Context, session auth.Session, sampleID string) (SampleDetailPageData, error) {
	sample, err := getSampleByID(sampleID)
	if err != nil {
		return SampleDetailPageData{}, err
	}

	data := SampleDetailPageData{
		BasePageData: BasePageData{Username: session.Username, UserID: session.UserID},
		Sample:       sample,
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

	// Parse form data
	err := r.ParseForm()
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
	owner := r.FormValue("owner")
	hasSamplePrep := r.Form.Has("sample_prep")
	sample_prep := r.FormValue("sample_prep")

	var (
		query string
		args  []interface{}
	)

	if hasSamplePrep {
		query = "UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3, sample_owner=$4, sample_prep=$6 WHERE sample_id=$5"
		args = []interface{}{name, description, keywords, owner, sampleID, sample_prep}
	} else {
		query = "UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3, sample_owner=$4 WHERE sample_id=$5"
		args = []interface{}{name, description, keywords, owner, sampleID}
	}

	_, err = dbPool.Exec(context.Background(), query, args...)
	if err != nil {
		if isHTMXRequest(r) {
			renderSampleEditSection(w, r, session, sampleID, "", "Failed to update the sample.")
			return
		}
		fmt.Println(err)
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

// newSampleFormHandler displays the form to add a new sample
// func newSampleHandler(w http.ResponseWriter, r *http.Request) {
//     if r.Method == http.MethodGet {
//         session := auth.MustSessionFromContext(r.Context())
//         data := struct {
//             BasePageData
//         }{
//             BasePageData: BasePageData{Username: session.Username},
//         }

//         tmpl, err := template.ParseFiles("templates/header.html", "templates/new_sample.html")
//         if err != nil {
//             http.Error(w, "Error loading template", http.StatusInternalServerError)
//             return
//         }
//         tmpl.ExecuteTemplate(w, "new_sample", data)
//         return
//     }
// 	tmpl.Execute(w, nil)
// }

// newSampleHandler handles both displaying the form and processing the submission
func newSampleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		session := auth.MustSessionFromContext(r.Context())
		data := struct {
			BasePageData
		}{
			BasePageData: BasePageData{Username: session.Username},
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
		owner := r.FormValue("owner")
		prep := r.FormValue("sample_prep")
		// Insert the new sample into the database
		_, err := dbPool.Exec(context.Background(),
			"INSERT INTO samples (sample_name, sample_description, sample_keywords, sample_prep, sample_owner) VALUES ($1, $2, $3, $4, $5)",
			name, description, keywords, prep, owner)
		if err != nil {
			fmt.Printf("%v", err)
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
	return attachments, nil
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

// Add these handler functions to your main.go:

// func uploadAttachmentHandler(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
// 		return
// 	}

// 	// Get sample ID from URL
// 	parts := strings.Split(r.URL.Path, "/")
// 	if len(parts) < 4 {
// 		http.Error(w, "Invalid URL", http.StatusBadRequest)
// 		return
// 	}
// 	sampleID := parts[3]

// 	// Parse multipart form
// 	err := r.ParseMultipartForm(10 << 20) // 10 MB max
// 	if err != nil {
// 		http.Error(w, "Error parsing form", http.StatusBadRequest)
// 		return
// 	}

// 	file, header, err := r.FormFile("file")
// 	if err != nil {
// 		http.Error(w, "Error retrieving file", http.StatusBadRequest)
// 		return
// 	}
// 	defer file.Close()

// 	// Save file and get filepath
// 	filepath, err := saveUploadedFile(file, header.Filename)
// 	if err != nil {
// 		http.Error(w, "Error saving file", http.StatusInternalServerError)
// 		return
// 	}

// 	// Add to database
// 	err = addAttachment(sampleID, filepath)
// 	if err != nil {
// 		http.Error(w, "Error storing attachment info", http.StatusInternalServerError)
// 		return
// 	}

// 	// Redirect back to sample detail page
// 	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
// }

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
	var filepath string
	err := dbPool.QueryRow(context.Background(),
		"SELECT attachment_address FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID,
	).Scan(&filepath)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	original := originalFilenameFromPath(filepath)
	setDownloadHeaders(w, original)
	// Serve the file
	http.ServeFile(w, r, resolveAppPath(filepath))
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
		if isHTMXRequest(r) {
			http.Error(w, "Attachment not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Attachment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error getting sample ID", http.StatusInternalServerError)
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

// Helper function to create a new user (you'll need to run this manually or create an admin interface)
func createUser(username, password string) error {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Insert user
	_, err = dbPool.Exec(context.Background(),
		"INSERT INTO users (username, password_hash, is_approved) VALUES ($1, $2, false)",
		username, string(hashedPassword))
	return err
}

// Helper function to approve a user (you'll need to run this manually or create an admin interface)
func approveUser(username string) error {
	_, err := dbPool.Exec(context.Background(),
		"UPDATE users SET is_approved = true WHERE username = $1",
		username)
	return err
}
