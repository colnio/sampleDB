package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"sampleDB/internal/auth"
)

// errSampleNotFound is returned by requireSampleOwner when the sample does not exist.
var errSampleNotFound = errors.New("sample not found")

// errForbidden is returned by requireSampleOwner when the user does not own the sample.
var errForbidden = errors.New("forbidden")

// generateShareToken creates or replaces the share token for a sample, setting
// the given visibility ('public' or 'authenticated'). Returns the new token.
func generateShareToken(sampleID int, visibility string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	token := hex.EncodeToString(b)
	_, err := dbPool.Exec(context.Background(),
		`UPDATE samples SET share_token=$1, share_visibility=$2 WHERE sample_id=$3`,
		token, visibility, sampleID)
	if err != nil {
		return "", fmt.Errorf("save share token: %w", err)
	}
	return token, nil
}

// getSampleByShareToken fetches a sample (with attachments) by its share token.
// Returns pgx.ErrNoRows if the token is not found.
func getSampleByShareToken(token string) (Sample, error) {
	var sample Sample
	err := dbPool.QueryRow(context.Background(),
		`SELECT s.sample_id, s.sample_name, COALESCE(s.sample_description,''), COALESCE(s.sample_keywords,''),
		        COALESCE(s.sample_owner,''), s.owner_user_id, u.username,
		        COALESCE(s.sample_prep,''), s.created_at,
		        s.share_token, COALESCE(s.share_visibility,'private')
		   FROM samples s
		   LEFT JOIN users u ON s.owner_user_id = u.user_id
		  WHERE s.share_token = $1`, token).Scan(
		&sample.ID, &sample.Name, &sample.Description, &sample.Keywords,
		&sample.Owner, &sample.OwnerUserID, &sample.OwnerUsername,
		&sample.Sample_prep, &sample.CreatedAt,
		&sample.ShareToken, &sample.ShareVisibility)
	if err != nil {
		return sample, err
	}
	var err2 error
	sample.Attachments, err2 = getAttachments(strconv.Itoa(sample.ID))
	if err2 != nil {
		return sample, err2
	}
	sample.SamplePrepHTML = renderMarkdown(sample.Sample_prep)
	return sample, nil
}

// revokeShareToken clears the share token and resets visibility to 'private'.
func revokeShareToken(sampleID int) error {
	_, err := dbPool.Exec(context.Background(),
		`UPDATE samples SET share_token=NULL, share_visibility='private' WHERE sample_id=$1`, sampleID)
	return err
}

// SharedSamplePageData is the template data for the public shared sample view.
type SharedSamplePageData struct {
	BasePageData
	Sample          Sample
	IsAuthenticated bool
	ShareURL        string
	Steps           []FabricationStep
}

// handleSharedSample serves GET /s/{token} — no auth middleware, visibility
// checked here. Public samples are served to anyone; authenticated samples
// require a valid session or Bearer token.
func handleSharedSample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimPrefix(r.URL.Path, "/s/")
	if token == "" {
		http.NotFound(w, r)
		return
	}

	sample, err := getSampleByShareToken(token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Error loading sample", http.StatusInternalServerError)
		}
		return
	}

	// Treat missing or 'private' visibility the same as not found.
	if sample.ShareVisibility == "private" || sample.ShareVisibility == "" {
		http.NotFound(w, r)
		return
	}

	// Optional auth — never redirects.
	session, isAuth := authManagerInstance.TryAuth(r)

	if sample.ShareVisibility == "authenticated" && !isAuth {
		// API clients (Bearer token present) get 403; browsers get redirected to login.
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "authentication required", http.StatusForbidden)
		} else {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
		return
	}

	sampleIDInt := sample.ID
	steps, _ := getFabricationSteps(sampleIDInt) // non-fatal

	data := SharedSamplePageData{
		Sample:          sample,
		IsAuthenticated: isAuth,
		ShareURL:        publicBaseURL() + "/s/" + token,
		Steps:           steps,
	}
	if isAuth {
		data.BasePageData = BasePageData{Username: session.Username, UserID: session.UserID}
	}

	tmpl, err := parseTemplates(
		"templates/sample_shared.html",
		"templates/fabrication_steps.html",
		"templates/step_form.html",
	)
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// handleSampleShare handles POST /samples/{id}/share — generates or replaces
// the share link for a sample.
func handleSampleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(pathParts) < 2 || pathParts[1] != "share" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	sampleID, err := strconv.Atoi(pathParts[0])
	if err != nil || sampleID <= 0 {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := requireSampleOwner(r, sampleID, session.UserID); err != nil {
		if errors.Is(err, errSampleNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
		} else {
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	visibility := r.FormValue("visibility")
	if visibility != "public" && visibility != "authenticated" {
		visibility = "public"
	}

	if _, err := generateShareToken(sampleID, visibility); err != nil {
		http.Error(w, "Error generating share token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/samples/%d", sampleID), http.StatusSeeOther)
}

// requireSampleOwner returns an error if the given userID does not own the sample.
// Samples with no owner_user_id set (legacy) are accessible to any authenticated user.
func requireSampleOwner(r *http.Request, sampleID, userID int) error {
	var ownerUserID *int
	err := dbPool.QueryRow(r.Context(),
		"SELECT owner_user_id FROM samples WHERE sample_id = $1",
		sampleID).Scan(&ownerUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errSampleNotFound
		}
		log.Printf("requireSampleOwner: db error: %v", err)
		return fmt.Errorf("database error")
	}
	if ownerUserID != nil && *ownerUserID != userID {
		return errForbidden
	}
	return nil
}

// handleSampleShareRevoke handles POST /samples/{id}/share/revoke.
func handleSampleShareRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(pathParts) < 3 || pathParts[1] != "share" || pathParts[2] != "revoke" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	sampleID, err := strconv.Atoi(pathParts[0])
	if err != nil || sampleID <= 0 {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := requireSampleOwner(r, sampleID, session.UserID); err != nil {
		if errors.Is(err, errSampleNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
		} else {
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
		return
	}

	if err := revokeShareToken(sampleID); err != nil {
		http.Error(w, "Error revoking share token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/samples/%d", sampleID), http.StatusSeeOther)
}
