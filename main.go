package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/auth"
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
	ID          int
	Name        string
	Description string
	Keywords    string
	Owner       string
	Sample_prep string
	Attachments []Attachment
}

type User struct {
	ID         int
	Username   string
	IsApproved bool
	CreatedAt  time.Time
}

type MainPageData struct {
	Samples  []Sample
	Username string
	Query    string
}

// Initialize a global database connection pool
var dbPool *pgxpool.Pool

var loc *time.Location

// Add this function to booking.go
func initLocation() {
	var err error
	loc, err = time.LoadLocation("Local")
	if err != nil {
		log.Fatalf("Failed to load local timezone: %v", err)
	}
}

func main() {
	initLocation()
	// Connect to PostgreSQL
	var err error
	dbURL := "postgres://app:app@localhost:5432/sampledb"
	dbPool, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbPool.Close()

	authManager := auth.NewManager(dbPool)

	// Set up static file serving
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Auth routes
	http.HandleFunc("/login", authManager.LoginHandler())
	http.HandleFunc("/register", authManager.RegisterHandler())
	http.HandleFunc("/logout", authManager.RequireAuth(authManager.LogoutHandler()))

	// Sample management routes
	withAuth := authManager.RequireAuth
	http.HandleFunc("/", withAuth(mainPageHandler))
	http.HandleFunc("/samples/new", withAuth(newSampleHandler))
	http.HandleFunc("/samples/edit/", withAuth(editSampleHandler))
	http.HandleFunc("/samples/", withAuth(handleSample))
	http.HandleFunc("/attachment/", withAuth(handleAttachment))
	http.HandleFunc("/booking", withAuth(handleBooking))
	http.HandleFunc("/api/bookings", withAuth(handleGetBookings))
	http.HandleFunc("/booking/delete", withAuth(handleDeleteBooking))
	// Wiki routes
	http.HandleFunc("/wiki", withAuth(handleWiki))
	http.HandleFunc("/wiki/", withAuth(handleWiki))                      // This will handle all wiki subpaths
	http.HandleFunc("/wiki/attachment/", withAuth(handleAttachmentWiki)) // This will handle all wiki subpaths

	http.HandleFunc("/admin", withAuth(requireAdmin(handleAdminPage)))
	http.HandleFunc("/admin/update-access", withAuth(requireAdmin(handleUpdateAccess)))
	http.HandleFunc("/admin/set-admin", withAuth(requireAdmin(handleSetAdmin)))
	http.HandleFunc("/admin/add-equipment", withAuth(requireAdmin(handleAddEquipment)))
	http.HandleFunc("/admin/delete-equipment/", withAuth(requireAdmin(handleDeleteEquipment))) // Note the trailing slash
	http.HandleFunc("/admin/equipment-report", withAuth(requireAdmin(handleEquipmentReport)))
	// Create uploads directory if it doesn't exist
	err = os.MkdirAll("uploads", 0755)
	if err != nil {
		log.Fatalf("Error creating uploads directory: %v\n", err)
	}

	// Create static directory if it doesn't exist
	err = os.MkdirAll("static/css", 0755)
	if err != nil {
		log.Fatalf("Error creating static directories: %v\n", err)
	}

	fmt.Println("Server started at :8010")
	log.Fatal(http.ListenAndServe(":8010", nil))
}

func parseTemplates(files ...string) (*template.Template, error) {
	// Always include base and header templates
	files = append([]string{
		"templates/base.html",
		"templates/header.html",
	}, files...)

	return template.ParseFiles(files...)
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
	rows := dbPool.QueryRow(context.Background(), `SELECT admin
	FROM users
	WHERE username = $1`, session.Username)
	if err != nil {
		fmt.Println(err)
		return
	}

	data := struct {
		BasePageData
		Samples []Sample
		Query   string
	}{
		BasePageData: BasePageData{Username: session.Username, IsAdmin: false},
		Samples:      samples,
		Query:        query,
	}

	err = rows.Scan(&data.BasePageData.IsAdmin)
	if err != nil {
		fmt.Println(err)
	}
	tmpl, err := parseTemplates("templates/main.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
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
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save file and get filepath
	filepath, err := saveUploadedFile(file, header.Filename)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	// Add to database
	err = addAttachment(sampleID, filepath)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Error storing attachment info", http.StatusInternalServerError)
		return
	}

	// Redirect back to sample detail page
	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
}

// searchSamples queries the database for samples by name or keywords
func searchSamples(query string) ([]Sample, error) {
	// Split the query into individual keywords
	keywords := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == ',' || r == ';'
	})

	// Construct the SQL query with `ANY` and `string_to_array`
	var whereClauses []string
	var args []interface{}
	for i, keyword := range keywords {
		whereClauses = append(whereClauses, fmt.Sprintf("$%d = ANY(string_to_array(lower(sample_keywords), ','))", i+1))
		args = append(args, strings.ToLower(keyword))
	}
	whereClause := strings.Join(whereClauses, " OR ")

	// Also match the full query string in `sample_name`
	queryText := fmt.Sprintf(`
        SELECT sample_id, sample_name, sample_description, sample_keywords, sample_owner
        FROM samples
        WHERE sample_name ILIKE $%d OR %s`, len(args)+1, whereClause)
	args = append(args, "%"+query+"%")

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

	sample, err := getSampleByID(sampleID)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}

	data := struct {
		BasePageData
		Sample Sample
	}{
		BasePageData: BasePageData{Username: session.Username},
		Sample:       sample,
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
	return sample, err
}

// editSampleHandler updates a sample's details in the database
func editSampleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract sample ID from URL by removing "/samples/edit/"
	sampleID := strings.TrimPrefix(r.URL.Path, "/samples/edit/")

	// Parse form data
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	keywords := r.FormValue("keywords")
	owner := r.FormValue("owner")
	sample_prep := r.FormValue("sample_prep")

	// fmt.Println(name, description, keywords, owner, sample_prep)
	// Update sample in the database
	_, err = dbPool.Exec(context.Background(),
		"UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3, sample_owner=$4, sample_prep=$6 WHERE sample_id=$5",
		name, description, keywords, owner, sampleID, sample_prep)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Error updating sample", http.StatusInternalServerError)
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

// generateUniqueFilename creates a unique filename with original extension
func generateUniqueFilename(originalName string) (string, error) {
	// Generate 16 random bytes
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Keep original file extension
	ext := filepath.Ext(originalName)
	return hex.EncodeToString(bytes) + ext, nil
}

// saveUploadedFile saves the file to disk and returns the file path
func saveUploadedFile(file io.Reader, originalName string) (string, error) {
	// Create uploads directory if it doesn't exist
	uploadDir := "uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}

	// Generate unique filename
	filename, err := generateUniqueFilename(originalName)
	if err != nil {
		return "", err
	}

	filepath := filepath.Join(uploadDir, filename)

	// Create new file
	dst, err := os.Create(filepath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	// Copy file content
	if _, err = io.Copy(dst, file); err != nil {
		return "", err
	}

	return filepath, nil
}

// getAttachments retrieves all attachments for a sample
func getAttachments(sampleID string) ([]Attachment, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT attachment_id, sample_id, attachment_address, uploaded_at 
         FROM attachments 
         WHERE sample_id = $1`,
		sampleID)
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
		// Extract original filename from path
		att.OriginalName = filepath.Base(att.Address)
		// Determine content type
		att.ContentType = mime.TypeByExtension(filepath.Ext(att.Address))
		att.IsImage = isImage(att.ContentType)
		attachments = append(attachments, att)
	}
	return attachments, nil
}

// addAttachment stores a new attachment in the database
func addAttachment(sampleID string, filepath string) error {
	_, err := dbPool.Exec(context.Background(),
		`INSERT INTO attachments (sample_id, attachment_address) 
         VALUES ($1, $2)`,
		sampleID, filepath)
	return err
}

// deleteAttachment removes an attachment from both database and filesystem
func deleteAttachment(attachmentID string) error {
	// First get the file path
	var filepath string
	err := dbPool.QueryRow(context.Background(),
		"SELECT attachment_address FROM attachments WHERE attachment_id = $1",
		attachmentID).Scan(&filepath)
	if err != nil {
		return err
	}

	// Delete from database
	_, err = dbPool.Exec(context.Background(),
		"DELETE FROM attachments WHERE attachment_id = $1",
		attachmentID)
	if err != nil {
		return err
	}

	// Delete file from filesystem
	return os.Remove(filepath)

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
		"SELECT attachment_address FROM attachments WHERE attachment_id = $1",
		attachmentID).Scan(&filepath)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	// Serve the file
	http.ServeFile(w, r, filepath)
}

func deleteAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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

	// Get sample ID before deleting the attachment
	var sampleID string
	err := dbPool.QueryRow(context.Background(),
		"SELECT sample_id FROM attachments WHERE attachment_id = $1",
		attachmentID).Scan(&sampleID)
	if err != nil {
		http.Error(w, "Error getting sample ID", http.StatusInternalServerError)
		return
	}

	// Delete the attachment
	err = deleteAttachment(attachmentID)
	if err != nil {
		http.Error(w, "Error deleting attachment", http.StatusInternalServerError)
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
