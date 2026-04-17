package samples

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// nonFileChars matches characters that are not safe for filenames.
var nonFileChars = regexp.MustCompile(`[^a-zA-Z0-9._\-]+`)

// HandlerConfig holds the configuration needed by the HTTP handlers.
type HandlerConfig struct {
	UploadsDir string
	BaseDir    string
	PublicHost string
	UseTLS     bool
}

// Handlers groups all HTTP handler methods for the samples domain.
type Handlers struct {
	svc      Service
	renderer *web.Renderer
	authMgr  *auth.Manager
	cfg      HandlerConfig
}

// NewHandlers constructs a Handlers value.
func NewHandlers(svc Service, r *web.Renderer, a *auth.Manager, cfg HandlerConfig) *Handlers {
	return &Handlers{svc: svc, renderer: r, authMgr: a, cfg: cfg}
}

// ------------------------------------------------------------------ utilities --

func isHTMXRequest(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("HX-Request"), "true")
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

func setContentDisposition(w http.ResponseWriter, disposition, filename string) {
	if filename == "" {
		return
	}
	safeName := strings.ReplaceAll(filename, "\"", "")
	safeName = strings.ReplaceAll(safeName, "\\", "")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("%s; filename=\"%s\"; filename*=UTF-8''%s",
			disposition,
			safeName, url.PathEscape(safeName)))
}

func setDownloadHeaders(w http.ResponseWriter, filename string) {
	setContentDisposition(w, "attachment", filename)
}

func setInlineHeaders(w http.ResponseWriter, filename string) {
	setContentDisposition(w, "inline", filename)
}

func (h *Handlers) resolveAppPath(path string) string {
	if path == "" {
		return path
	}
	if h.cfg.BaseDir == "" {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Clean(path)
	}
	var joined string
	if filepath.IsAbs(path) {
		joined = filepath.Clean(path)
	} else {
		joined = filepath.Join(h.cfg.BaseDir, path)
	}
	rel, err := filepath.Rel(h.cfg.BaseDir, joined)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return joined
}

func (h *Handlers) publicBaseURL() string {
	if h.cfg.UseTLS {
		return "https://" + h.cfg.PublicHost
	}
	return "http://" + h.cfg.PublicHost
}

// saveUploadedFile saves the multipart-uploaded file to disk and returns the
// relative path (relative to BaseDir when available).
func (h *Handlers) saveUploadedFile(file io.Reader, originalName string) (string, error) {
	uploadDir := h.cfg.UploadsDir
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}
	filename, err := generateUniqueFilename(originalName)
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(uploadDir, filename)
	dst, err := os.Create(fullPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err = io.Copy(dst, file); err != nil {
		return "", err
	}
	if h.cfg.BaseDir != "" {
		if rel, err := filepath.Rel(h.cfg.BaseDir, fullPath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel), nil
		}
	}
	return fullPath, nil
}

// handleOwnerError writes the appropriate HTTP error for ownership check failures.
func (h *Handlers) handleOwnerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, web.ErrNotFound):
		http.Error(w, "Sample not found", http.StatusNotFound)
	case errors.Is(err, web.ErrForbidden):
		http.Error(w, "Forbidden", http.StatusForbidden)
	default:
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// --------------------------------------------------------- internal helpers --

// loadSampleDetailData builds a SampleDetailPageData for the given sampleIDStr.
func (h *Handlers) loadSampleDetailData(ctx context.Context, session auth.Session, sampleIDStr string) (SampleDetailPageData, error) {
	sampleID, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		return SampleDetailPageData{}, web.ErrNotFound
	}

	sample, err := h.svc.GetByID(ctx, sampleID)
	if err != nil {
		return SampleDetailPageData{}, err
	}
	sample.SamplePrepHTML = web.RenderMarkdown(sample.SamplePrep)

	users, err := h.svc.GetApprovedUsers(ctx)
	if err != nil {
		return SampleDetailPageData{}, err
	}

	equipment, err := h.svc.GetAllEquipment(ctx)
	if err != nil {
		return SampleDetailPageData{}, err
	}

	steps, _ := h.svc.GetSteps(ctx, sampleID) // non-fatal: show empty on error

	shareURL := ""
	if sample.ShareToken != nil && *sample.ShareToken != "" {
		shareURL = h.publicBaseURL() + "/s/" + *sample.ShareToken
	}

	isAdmin, _ := h.svc.IsAdmin(ctx, session.UserID)

	return SampleDetailPageData{
		BasePageData: BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Sample:       sample,
		Users:        users,
		Equipment:    equipment,
		Steps:        steps,
		ShareURL:     shareURL,
	}, nil
}

// renderSampleAttachmentsSection re-renders the attachments panel as an HTMX partial.
func (h *Handlers) renderSampleAttachmentsSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleID, flash, errMsg string) {
	data, err := h.loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true
	if err := h.renderer.RenderSection(w, "sample_detail", "sample_attachments", data); err != nil {
		http.Error(w, "Error rendering attachments", http.StatusInternalServerError)
	}
}

// renderSampleEditSection re-renders the edit form panel as an HTMX partial.
func (h *Handlers) renderSampleEditSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleID, flash, errMsg string) {
	data, err := h.loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true
	if err := h.renderer.RenderSection(w, "sample_detail", "sample_edit_form", data); err != nil {
		http.Error(w, "Error rendering form", http.StatusInternalServerError)
	}
}

// renderFabricationStepsSection re-renders the fabrication steps panel as an HTMX partial.
func (h *Handlers) renderFabricationStepsSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleIDStr, flash, errMsg string, editingStep int) {
	data, err := h.loadSampleDetailData(r.Context(), session, sampleIDStr)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true
	data.EditingStep = editingStep
	if err := h.renderer.RenderSection(w, "sample_detail", "fabrication_steps_panel", data); err != nil {
		http.Error(w, "Error rendering steps panel", http.StatusInternalServerError)
	}
}

// --------------------------------------------------------- HTML handlers --

// MainPage serves GET / — the main sample list / search page.
func (h *Handlers) MainPage(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	dateFrom := r.URL.Query().Get("date_from")
	dateTo := r.URL.Query().Get("date_to")

	var (
		samples []Sample
		err     error
	)
	if query != "" {
		samples, err = h.svc.Search(r.Context(), query, dateFrom, dateTo)
	} else {
		samples, err = h.svc.ListAll(r.Context(), dateFrom, dateTo)
	}
	if err != nil {
		http.Error(w, "Error retrieving samples", http.StatusInternalServerError)
		return
	}

	isAdmin, _ := h.svc.IsAdmin(r.Context(), session.UserID)

	data := MainPageData{
		BasePageData: BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Samples:      samples,
		Query:        query,
		DateFrom:     dateFrom,
		DateTo:       dateTo,
	}

	if isHTMXRequest(r) && r.Header.Get("HX-Target") == "samples-panel" {
		if err := h.renderer.RenderSection(w, "main", "samples_panel", data); err != nil {
			http.Error(w, "Error rendering samples", http.StatusInternalServerError)
		}
		return
	}

	if err := h.renderer.Render(w, "main", data); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// NewSample handles GET /samples/new (render form) and POST /samples/new (create).
func (h *Handlers) NewSample(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		session := auth.MustSessionFromContext(r.Context())
		isAdmin, _ := h.svc.IsAdmin(r.Context(), session.UserID)
		users, err := h.svc.GetApprovedUsers(r.Context())
		if err != nil {
			http.Error(w, "Error loading user list", http.StatusInternalServerError)
			return
		}
		data := NewSamplePageData{
			BasePageData: BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
			Users:        users,
		}
		if err := h.renderer.Render(w, "new_sample", data); err != nil {
			http.Error(w, "Error rendering page", http.StatusInternalServerError)
		}

	case http.MethodPost:
		session := auth.MustSessionFromContext(r.Context())
		isAdmin, _ := h.svc.IsAdmin(r.Context(), session.UserID)

		name := r.FormValue("name")
		description := r.FormValue("description")
		keywords := r.FormValue("keywords")

		var ownerUserID *int
		var ownerName string
		if !isAdmin {
			uid := session.UserID
			ownerUserID = &uid
			ownerName, _ = h.svc.GetUsernameByID(r.Context(), session.UserID)
		} else {
			if s := r.FormValue("owner_user_id"); s != "" {
				id, err := strconv.Atoi(s)
				if err == nil {
					ownerUserID = &id
					ownerName, _ = h.svc.GetUsernameByID(r.Context(), id)
				}
			}
		}

		if strings.TrimSpace(name) == "" {
			http.Error(w, "Sample name is required", http.StatusBadRequest)
			return
		}

		newID, err := h.svc.Create(r.Context(), CreateParams{
			Name:        name,
			Description: description,
			Keywords:    keywords,
			Owner:       ownerName,
			OwnerUserID: ownerUserID,
		})
		if err != nil {
			log.Printf("samples.NewSample: insert error: %v", err)
			http.Error(w, "Error adding sample", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/samples/%d", newID), http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// EditSample handles POST /samples/edit/{id}.
func (h *Handlers) EditSample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	sampleID := strings.TrimPrefix(r.URL.Path, "/samples/edit/")
	sampleIDInt, err := strconv.Atoi(sampleID)
	if err != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.CheckOwner(r.Context(), sampleIDInt, session.UserID); err != nil {
		if isHTMXRequest(r) {
			h.renderSampleEditSection(w, r, session, sampleID, "", "You do not have permission to edit this sample.")
			return
		}
		h.handleOwnerError(w, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		if isHTMXRequest(r) {
			h.renderSampleEditSection(w, r, session, sampleID, "", "Unable to read the form submission.")
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	keywords := r.FormValue("keywords")

	isAdmin, _ := h.svc.IsAdmin(r.Context(), session.UserID)

	var ownerUserID *int
	var ownerName string
	if isAdmin {
		if s := r.FormValue("owner_user_id"); s != "" {
			id, parseErr := strconv.Atoi(s)
			if parseErr != nil {
				http.Error(w, "invalid owner_user_id", http.StatusBadRequest)
				return
			}
			uname, scanErr := h.svc.GetUsernameByIDApproved(r.Context(), id)
			if scanErr != nil {
				http.Error(w, "owner user not found or not approved", http.StatusBadRequest)
				return
			}
			ownerUserID = &id
			ownerName = uname
		} else {
			// No owner_user_id submitted — preserve the existing owner rather
			// than silently wiping it by passing nil to Update.
			existing, fetchErr := h.svc.GetByID(r.Context(), sampleIDInt)
			if fetchErr == nil {
				ownerUserID = existing.OwnerUserID
				ownerName = existing.Owner
			}
		}
	} else {
		uid := session.UserID
		ownerUserID = &uid
		ownerName, _ = h.svc.GetUsernameByID(r.Context(), session.UserID)
	}

	if err := h.svc.Update(r.Context(), sampleIDInt, UpdateParams{
		Name:        name,
		Description: description,
		Keywords:    keywords,
		Owner:       ownerName,
		OwnerUserID: ownerUserID,
	}); err != nil {
		if isHTMXRequest(r) {
			h.renderSampleEditSection(w, r, session, sampleID, "", "Failed to update the sample.")
			return
		}
		log.Printf("samples.EditSample: update error: %v", err)
		http.Error(w, "Error updating sample", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		h.renderSampleEditSection(w, r, session, sampleID, "Changes saved", "")
		return
	}
	http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
}

// DeleteSample handles POST /samples/{id}/delete. Admin-only: removes the
// sample row, its attachment rows, and the attachment files on disk. The
// cascade is run inside a transaction by svc.APIDeleteSample, so either the
// whole sample disappears or nothing changes.
func (h *Handlers) DeleteSample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("samples.DeleteSample: IsAdmin error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if !isAdmin {
		http.Error(w, "Forbidden: admin privileges required", http.StatusForbidden)
		return
	}

	// URL: /samples/{id}/delete  →  pathParts after /samples/ is [id, "delete"]
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(parts) != 2 || parts[1] != "delete" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	sampleID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	filePaths, err := h.svc.APIDeleteSample(r.Context(), sampleID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
			return
		}
		log.Printf("samples.DeleteSample: APIDeleteSample: %v", err)
		http.Error(w, "Failed to delete sample", http.StatusInternalServerError)
		return
	}

	// Remove attachment files from disk (best-effort; DB row is already gone).
	for _, fp := range filePaths {
		if fp == "" {
			continue
		}
		if removeErr := removeFile(h.resolveAppPath(fp)); removeErr != nil {
			log.Printf("samples.DeleteSample: remove file %q: %v", fp, removeErr)
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// SampleDetail handles GET /samples/{id}.
func (h *Handlers) sampleDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := auth.MustSessionFromContext(r.Context())
	sampleID := strings.TrimPrefix(r.URL.Path, "/samples/")

	data, err := h.loadSampleDetailData(r.Context(), session, sampleID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Sample not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error loading sample", http.StatusInternalServerError)
		}
		return
	}

	if err := h.renderer.Render(w, "sample_detail", data); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// HandleSample dispatches requests under /samples/{id}[/...].
func (h *Handlers) HandleSample(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(pathParts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	switch {
	case len(pathParts) == 2 && pathParts[1] == "upload":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed for upload", http.StatusMethodNotAllowed)
			return
		}
		h.uploadAttachment(w, r)

	case len(pathParts) == 3 && pathParts[1] == "steps" && pathParts[2] == "reorder":
		h.reorderSteps(w, r)

	case len(pathParts) == 2 && pathParts[1] == "steps":
		h.stepsEndpoint(w, r)

	case len(pathParts) == 3 && pathParts[1] == "share" && pathParts[2] == "revoke":
		h.handleShareRevoke(w, r)

	case len(pathParts) == 2 && pathParts[1] == "share":
		h.handleShare(w, r)

	case len(pathParts) == 2 && pathParts[1] == "delete":
		h.DeleteSample(w, r)

	case len(pathParts) == 1:
		h.sampleDetail(w, r)

	default:
		http.Error(w, "Invalid URL", http.StatusBadRequest)
	}
}

// HandleAttachment dispatches GET (download) and POST /delete (delete) for /attachment/{id}.
func (h *Handlers) HandleAttachment(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/delete") {
		h.deleteAttachment(w, r)
		return
	}
	if r.Method == http.MethodGet {
		h.downloadAttachment(w, r)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// downloadAttachment handles GET /attachment/{id}.
func (h *Handlers) downloadAttachment(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	attachmentID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	filePath, err := h.svc.AttachmentFilePath(r.Context(), attachmentID)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	resolved := h.resolveAppPath(filePath)
	if resolved == "" {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}
	original := originalFilenameFromPath(filePath)
	if !isImage(mime.TypeByExtension(filepath.Ext(filePath))) {
		setDownloadHeaders(w, original)
	}
	http.ServeFile(w, r, resolved)
}

// deleteAttachment handles POST /attachment/{id}/delete.
func (h *Handlers) deleteAttachment(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	attachmentID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid attachment ID", http.StatusBadRequest)
		return
	}

	sampleID, _, err := h.svc.AttachmentInfo(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Attachment not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to remove attachment.", http.StatusInternalServerError)
		return
	}
	sampleIDStr := strconv.Itoa(sampleID)

	if ownerErr := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); ownerErr != nil {
		if isHTMXRequest(r) {
			h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, "", "You do not have permission to delete this attachment.")
			return
		}
		h.handleOwnerError(w, ownerErr)
		return
	}

	filePath, err := h.svc.DeleteAttachment(r.Context(), attachmentID)
	if err != nil {
		if isHTMXRequest(r) {
			h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, "", "Failed to remove attachment.")
			return
		}
		http.Error(w, "Error deleting attachment", http.StatusInternalServerError)
		return
	}
	if filePath != "" {
		if removeErr := os.Remove(h.resolveAppPath(filePath)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			log.Printf("samples.deleteAttachment: file remove error: %v", removeErr)
		}
	}

	if isHTMXRequest(r) {
		h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, "Attachment removed", "")
		return
	}
	http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)
}

// uploadAttachment handles POST /samples/{id}/upload.
func (h *Handlers) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	if len(pathParts) != 2 || pathParts[1] != "upload" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	sampleIDStr := pathParts[0]
	sampleID, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); err != nil {
		if isHTMXRequest(r) {
			h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, "", "You do not have permission to upload attachments.")
			return
		}
		h.handleOwnerError(w, err)
		return
	}

	const maxUploadBytes = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if isHTMXRequest(r) {
			h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, "", "Unable to read the upload form.")
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		if isHTMXRequest(r) {
			h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, "", "Please choose at least one file to upload.")
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
		fp, saveErr := h.saveUploadedFile(f, fh.Filename)
		f.Close()
		if saveErr != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("could not save %s", fh.Filename))
			continue
		}
		if dbErr := h.svc.AddAttachment(r.Context(), sampleID, fp); dbErr != nil {
			_ = os.Remove(h.resolveAppPath(fp))
			errMsgs = append(errMsgs, fmt.Sprintf("could not store %s", fh.Filename))
			continue
		}
		uploaded++
	}

	flash := ""
	errMsg := ""
	if uploaded > 0 {
		flash = fmt.Sprintf("%d file(s) uploaded", uploaded)
	}
	if len(errMsgs) > 0 {
		errMsg = strings.Join(errMsgs, "; ")
	}

	if isHTMXRequest(r) {
		h.renderSampleAttachmentsSection(w, r, session, sampleIDStr, flash, errMsg)
		return
	}
	http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)
}

// --------------------------------------------------------- Sharing handlers --

// SharedSample handles GET /s/{token} — the public shared sample view.
func (h *Handlers) SharedSample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/s/")
	if rest == "" {
		http.NotFound(w, r)
		return
	}
	pathParts := strings.SplitN(rest, "/", 3)
	token := pathParts[0]
	if token == "" {
		http.NotFound(w, r)
		return
	}

	if len(pathParts) == 3 && pathParts[1] == "attachment" {
		h.serveSharedAttachment(w, r, token, pathParts[2])
		return
	}

	sample, err := h.svc.GetByShareToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Error loading sample", http.StatusInternalServerError)
		}
		return
	}

	if sample.ShareVisibility == "private" || sample.ShareVisibility == "" {
		http.NotFound(w, r)
		return
	}

	session, isAuth, tryAuthErr := h.authMgr.TryAuth(r)
	if tryAuthErr != nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	if sample.ShareVisibility == "authenticated" && !isAuth {
		authHdr := r.Header.Get("Authorization")
		if len(authHdr) > 7 && strings.EqualFold(authHdr[:7], "bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"authentication required"}`)
		} else {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		}
		return
	}

	steps, stepsErr := h.svc.GetSteps(r.Context(), sample.ID)
	if stepsErr != nil {
		log.Printf("samples.SharedSample: GetSteps error for sample %d: %v", sample.ID, stepsErr)
	}

	data := SharedSamplePageData{
		Sample:          sample,
		IsAuthenticated: isAuth,
		ShareURL:        h.publicBaseURL() + "/s/" + token,
		ShareToken:      token,
		Steps:           steps,
	}
	if isAuth {
		data.BasePageData = BasePageData{Username: session.Username, UserID: session.UserID}
	}

	if err := h.renderer.Render(w, "sample_shared", data); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// serveSharedAttachment serves GET /s/{token}/attachment/{id}.
func (h *Handlers) serveSharedAttachment(w http.ResponseWriter, r *http.Request, token, attachmentID string) {
	_, visibility, filePath, err := h.svc.SharedAttachmentInfo(r.Context(), token, attachmentID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	if visibility == "private" || visibility == "" {
		http.NotFound(w, r)
		return
	}

	if visibility == "authenticated" {
		_, ok, tryAuthErr := h.authMgr.TryAuth(r)
		if tryAuthErr != nil {
			http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
			return
		}
		if !ok {
			authHdr := r.Header.Get("Authorization")
			if len(authHdr) > 7 && strings.EqualFold(authHdr[:7], "bearer ") {
				w.Header().Set("WWW-Authenticate", "Bearer")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"authentication required"}`)
			} else {
				http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			}
			return
		}
	}

	resolved := h.resolveAppPath(filePath)
	if resolved == "" {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}
	original := originalFilenameFromPath(filePath)
	if !isImage(mime.TypeByExtension(filepath.Ext(filePath))) {
		setDownloadHeaders(w, original)
	}
	http.ServeFile(w, r, resolved)
}

// handleShare handles POST /samples/{id}/share.
func (h *Handlers) handleShare(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); err != nil {
		h.handleOwnerError(w, err)
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

	if _, err := h.svc.GenerateShareToken(r.Context(), sampleID, visibility); err != nil {
		http.Error(w, "Error generating share token", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/samples/%d", sampleID), http.StatusSeeOther)
}

// handleShareRevoke handles POST /samples/{id}/share/revoke.
func (h *Handlers) handleShareRevoke(w http.ResponseWriter, r *http.Request) {
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
	if err := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); err != nil {
		h.handleOwnerError(w, err)
		return
	}

	if err := h.svc.RevokeShareToken(r.Context(), sampleID); err != nil {
		http.Error(w, "Error revoking share token", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/samples/%d", sampleID), http.StatusSeeOther)
}

// ------------------------------------------------ Fabrication step handlers --

// parseStepForm reads step fields from a parsed form.
func parseStepForm(r *http.Request) (FabricationStep, error) {
	var step FabricationStep

	if s := r.FormValue("step_order"); s != "" {
		order, err := strconv.Atoi(s)
		if err != nil {
			return FabricationStep{}, fmt.Errorf("invalid step order")
		}
		step.StepOrder = order
	}

	if s := r.FormValue("step_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return FabricationStep{}, fmt.Errorf("invalid date: use YYYY-MM-DD")
		}
		step.StepDate = &t
	}

	if s := r.FormValue("person_user_id"); s != "" {
		id, err := strconv.Atoi(s)
		if err == nil {
			step.PersonUserID = &id
		}
	}

	if s := r.FormValue("equipment_id"); s != "" {
		id, err := strconv.Atoi(s)
		if err == nil {
			step.EquipmentID = &id
		}
	}

	if s := strings.TrimSpace(r.FormValue("equipment_text")); s != "" {
		step.EquipmentText = &s
	}

	step.Description = r.FormValue("description")
	return step, nil
}

// StepAction handles /steps/{id}/edit and /steps/{id}/delete.
func (h *Handlers) StepAction(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/steps/"), "/")
	if trimmed == "" {
		http.Error(w, "Step ID required", http.StatusBadRequest)
		return
	}

	parts := strings.Split(trimmed, "/")
	stepIDStr := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch action {
	case "edit":
		h.stepEdit(w, r, stepIDStr)
	case "delete":
		h.stepDelete(w, r, stepIDStr)
	default:
		http.Error(w, "Unknown step action", http.StatusNotFound)
	}
}

// stepsEndpoint handles GET and POST for /samples/{id}/steps.
func (h *Handlers) stepsEndpoint(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	sampleIDStr := parts[0]

	switch r.Method {
	case http.MethodGet:
		h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "", 0)

	case http.MethodPost:
		sampleID, err := strconv.Atoi(sampleIDStr)
		if err != nil {
			http.Error(w, "Invalid sample ID", http.StatusBadRequest)
			return
		}

		if err := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); err != nil {
			h.handleOwnerError(w, err)
			return
		}

		if err := r.ParseForm(); err != nil {
			h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Invalid form submission.", 0)
			return
		}

		step, parseErr := parseStepForm(r)
		if parseErr != nil {
			h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", parseErr.Error(), 0)
			return
		}

		if _, err := h.svc.AddStep(r.Context(), sampleID, step); err != nil {
			h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Failed to add step.", 0)
			return
		}

		if isHTMXRequest(r) {
			h.renderFabricationStepsSection(w, r, session, sampleIDStr, "Step added", "", 0)
			return
		}
		http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// stepEdit handles GET (inline edit mode) and POST (save update) for /steps/{id}/edit.
func (h *Handlers) stepEdit(w http.ResponseWriter, r *http.Request, stepIDStr string) {
	session := auth.MustSessionFromContext(r.Context())
	stepID, err := strconv.Atoi(stepIDStr)
	if err != nil {
		http.Error(w, "Invalid step ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		existing, err := h.svc.GetStepByID(r.Context(), stepID)
		if err != nil {
			if errors.Is(err, web.ErrNotFound) {
				http.Error(w, "Step not found", http.StatusNotFound)
			} else {
				log.Printf("samples.stepEdit GET: GetStepByID error: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if err := h.svc.CheckOwner(r.Context(), existing.SampleID, session.UserID); err != nil {
			h.handleOwnerError(w, err)
			return
		}
		sampleIDStr := strconv.Itoa(existing.SampleID)
		h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "", stepID)

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}

		existing, err := h.svc.GetStepByID(r.Context(), stepID)
		if err != nil {
			if errors.Is(err, web.ErrNotFound) {
				http.Error(w, "Step not found", http.StatusNotFound)
			} else {
				log.Printf("samples.stepEdit POST: GetStepByID error: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if err := h.svc.CheckOwner(r.Context(), existing.SampleID, session.UserID); err != nil {
			h.handleOwnerError(w, err)
			return
		}

		step, parseErr := parseStepForm(r)
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}

		if err := h.svc.UpdateStep(r.Context(), stepID, step); err != nil {
			http.Error(w, "Failed to update step", http.StatusInternalServerError)
			return
		}

		sampleIDStr := strconv.Itoa(existing.SampleID)
		if isHTMXRequest(r) {
			h.renderFabricationStepsSection(w, r, session, sampleIDStr, "Step updated", "", 0)
			return
		}
		http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// stepDelete handles POST /steps/{id}/delete.
func (h *Handlers) stepDelete(w http.ResponseWriter, r *http.Request, stepIDStr string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	stepID, err := strconv.Atoi(stepIDStr)
	if err != nil {
		http.Error(w, "Invalid step ID", http.StatusBadRequest)
		return
	}

	existing, err := h.svc.GetStepByID(r.Context(), stepID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Step not found", http.StatusNotFound)
		} else {
			log.Printf("samples.stepDelete: GetStepByID error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	if err := h.svc.CheckOwner(r.Context(), existing.SampleID, session.UserID); err != nil {
		h.handleOwnerError(w, err)
		return
	}
	sampleIDStr := strconv.Itoa(existing.SampleID)

	if err := h.svc.DeleteStep(r.Context(), stepID); err != nil {
		h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Failed to delete step.", 0)
		return
	}

	if isHTMXRequest(r) {
		h.renderFabricationStepsSection(w, r, session, sampleIDStr, "Step deleted", "", 0)
		return
	}
	http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)
}

// reorderSteps handles POST /samples/{id}/steps/reorder.
func (h *Handlers) reorderSteps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	sampleIDStr := parts[0]
	sampleID, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); err != nil {
		log.Printf("samples.reorderSteps: ownership check failed for sample %d user %d: %v", sampleID, session.UserID, err)
		h.handleOwnerError(w, err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}

	rawIDs := r.Form["step_id"]
	orderedIDs := make([]int, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := strconv.Atoi(raw)
		if err != nil {
			http.Error(w, "Invalid step ID in order list", http.StatusBadRequest)
			return
		}
		orderedIDs = append(orderedIDs, id)
	}

	if err := h.svc.ReorderSteps(r.Context(), sampleID, orderedIDs); err != nil {
		h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Failed to reorder steps.", 0)
		return
	}

	if isHTMXRequest(r) {
		h.renderFabricationStepsSection(w, r, session, sampleIDStr, "", "", 0)
		return
	}
	http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)
}
