package wiki

import (
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
	"strings"

	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// nonFileChars matches characters that are not safe for filenames.
var nonFileChars = regexp.MustCompile(`[^a-zA-Z0-9._\-]+`)

// HandlerConfig holds the configuration needed by the HTTP handlers.
type HandlerConfig struct {
	UploadsDir string
	BaseDir    string
}

// Handlers groups all HTTP handler methods for the wiki domain.
type Handlers struct {
	svc      Service
	renderer *web.Renderer
	cfg      HandlerConfig
}

// NewHandlers constructs a Handlers value.
func NewHandlers(svc Service, r *web.Renderer, cfg HandlerConfig) *Handlers {
	return &Handlers{svc: svc, renderer: r, cfg: cfg}
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

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

func setDownloadHeaders(w http.ResponseWriter, filename string) {
	if filename == "" {
		return
	}
	safeName := strings.ReplaceAll(filename, "\"", "")
	safeName = strings.ReplaceAll(safeName, "\\", "")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=\"%s\"; filename*=UTF-8''%s",
			safeName, url.PathEscape(safeName)))
}

// resolveAppPath converts a relative storage path to an absolute filesystem
// path using cfg.BaseDir.  Returns "" if the resolved path would escape the
// base directory.
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

// saveUploadedFile writes the uploaded file to disk and returns the storage
// path (relative to BaseDir when possible).
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

// ---------------------------------------------------------------------------
// Route dispatcher
// ---------------------------------------------------------------------------

// HandleWiki dispatches incoming /wiki[/...] requests.
func (h *Handlers) HandleWiki(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/wiki" || r.URL.Path == "/wiki/" {
		h.listArticles(w, r)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/wiki/"), "/")
	if len(pathParts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	switch pathParts[0] {
	case "new":
		h.newArticle(w, r)
	case "edit":
		h.editArticle(w, r)
	case "delete":
		h.deleteArticle(w, r)
	case "upload":
		h.uploadAttachment(w, r)
	case "preview":
		h.Preview(w, r)
	case "view":
		if len(pathParts) < 2 || pathParts[1] == "" {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		title, err := url.PathUnescape(pathParts[1])
		if err != nil {
			http.Error(w, "Invalid article title", http.StatusBadRequest)
			return
		}
		h.viewArticle(w, r, title)
	default:
		// Backward compatibility: /wiki/{title} -> /wiki/view/{title}
		http.Redirect(w, r, "/wiki/view/"+url.PathEscape(pathParts[0]), http.StatusSeeOther)
	}
}

// HandleAttachment dispatches /wiki/attachment/{id}[/delete] requests.
func (h *Handlers) HandleAttachment(w http.ResponseWriter, r *http.Request) {
	trimmedPath := strings.TrimSuffix(r.URL.Path, "/")
	pathParts := strings.Split(trimmedPath, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	attachmentID := pathParts[3]

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(trimmedPath, "/delete"):
		h.deleteAttachment(w, r, attachmentID)
	case r.Method == http.MethodGet:
		h.downloadAttachment(w, r, attachmentID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// HTML handlers
// ---------------------------------------------------------------------------

func (h *Handlers) listArticles(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	articles, err := h.svc.ListArticles(r.Context())
	if err != nil {
		http.Error(w, "Error retrieving articles", http.StatusInternalServerError)
		return
	}

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("wiki: unable to load admin flag for %s: %v", session.Username, err)
	}

	data := WikiListPageData{
		BasePageData: BasePageData{Username: session.Username, IsAdmin: isAdmin},
		Articles:     articles,
	}

	if err := h.renderer.Render(w, "wiki_list", data); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

func (h *Handlers) viewArticle(w http.ResponseWriter, r *http.Request, title string) {
	session := auth.MustSessionFromContext(r.Context())

	if isHTMXRequest(r) && r.Method == http.MethodGet {
		target := r.Header.Get("HX-Target")
		if target == "article-content-panel" {
			h.renderContentPanel(w, r, session, title, "", "", false)
			return
		}
	}

	article, err := h.svc.GetArticle(r.Context(), title)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Article not found", http.StatusNotFound)
		} else {
			log.Printf("wiki: viewArticle: %v", err)
			http.Error(w, "Error loading article", http.StatusInternalServerError)
		}
		return
	}

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("wiki: unable to load admin status for user %d: %v", session.UserID, err)
	}

	data := WikiViewPageData{
		BasePageData:   BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Article:        article,
		EditingContent: false,
	}

	if err := h.renderer.Render(w, "wiki_view", data); err != nil {
		log.Printf("wiki: error rendering view template: %v", err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

func (h *Handlers) renderContentPanel(w http.ResponseWriter, r *http.Request, session auth.Session, title, flash, errMsg string, editing bool) {
	article, err := h.svc.GetArticle(r.Context(), title)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Article not found", http.StatusNotFound)
		} else {
			log.Printf("wiki: renderContentPanel: %v", err)
			http.Error(w, "Error loading article", http.StatusInternalServerError)
		}
		return
	}

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("wiki: unable to load admin status for user %d: %v", session.UserID, err)
	}

	data := WikiViewPageData{
		BasePageData:   BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Article:        article,
		EditingContent: editing,
		IsPartial:      true,
		Flash:          flash,
		Error:          errMsg,
	}

	if err := h.renderer.RenderSection(w, "wiki_view", "article_content_panel", data); err != nil {
		http.Error(w, "Error rendering article content", http.StatusInternalServerError)
	}
}

func (h *Handlers) newArticle(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	if r.Method == http.MethodGet {
		data := WikiEditPageData{
			BasePageData: BasePageData{Username: session.Username},
		}
		if err := h.renderer.Render(w, "wiki_edit", data); err != nil {
			http.Error(w, "Error rendering page", http.StatusInternalServerError)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	title := r.FormValue("title")
	if strings.TrimSpace(title) == "" {
		http.Error(w, "Article title is required", http.StatusBadRequest)
		return
	}
	content := r.FormValue("content")

	if err := h.svc.CreateArticle(r.Context(), title, content, session.UserID); err != nil {
		log.Printf("wiki: error creating article %q: %v", title, err)
		http.Error(w, "Error creating article", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/wiki/view/"+url.PathEscape(title), http.StatusSeeOther)
}

func (h *Handlers) editArticle(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/wiki/edit/"), "/")
	if len(pathParts) < 1 || pathParts[0] == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	title, err := url.PathUnescape(pathParts[0])
	if err != nil {
		http.Error(w, "Invalid article title", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		if isHTMXRequest(r) {
			h.renderContentPanel(w, r, session, title, "", "", true)
			return
		}

		article, err := h.svc.GetArticle(r.Context(), title)
		if err != nil {
			http.Error(w, "Article not found", http.StatusNotFound)
			return
		}

		var articleImages []ArticleAttachment
		for _, att := range article.Attachments {
			if att.IsImage {
				articleImages = append(articleImages, att)
			}
		}

		data := WikiEditPageData{
			BasePageData:  BasePageData{Username: session.Username},
			Article:       article,
			ArticleImages: articleImages,
		}
		if err := h.renderer.Render(w, "wiki_edit", data); err != nil {
			http.Error(w, "Error rendering page", http.StatusInternalServerError)
		}
		return
	}

	// POST
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseForm(); err != nil {
		if isHTMXRequest(r) {
			h.renderContentPanel(w, r, session, title, "", "Invalid form submission.", true)
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	createdBy, isAdmin, authErr := h.svc.ArticleEditAccess(r.Context(), title, session.UserID)
	if authErr != nil {
		if errors.Is(authErr, web.ErrNotFound) {
			if isHTMXRequest(r) {
				h.renderContentPanel(w, r, session, title, "", "Article not found.", true)
				return
			}
			http.Error(w, "Article not found", http.StatusNotFound)
			return
		}
		if isHTMXRequest(r) {
			h.renderContentPanel(w, r, session, title, "", "Error checking article permissions.", true)
			return
		}
		http.Error(w, "Error checking permissions", http.StatusInternalServerError)
		return
	}
	if createdBy != session.UserID && !isAdmin {
		if isHTMXRequest(r) {
			h.renderContentPanel(w, r, session, title, "", "You do not have permission to edit this article.", true)
			return
		}
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	content := r.FormValue("content")
	if execErr := h.svc.UpdateArticleContent(r.Context(), title, content, session.UserID); execErr != nil {
		if isHTMXRequest(r) {
			h.renderContentPanel(w, r, session, title, "", "Failed to update article content.", true)
			return
		}
		http.Error(w, "Error updating article", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		h.renderContentPanel(w, r, session, title, "Content updated", "", false)
		return
	}

	http.Redirect(w, r, "/wiki/view/"+url.PathEscape(title), http.StatusSeeOther)
}

func (h *Handlers) deleteArticle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 || parts[3] == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	title, err := url.PathUnescape(parts[3])
	if err != nil {
		http.Error(w, "Invalid article title", http.StatusBadRequest)
		return
	}

	createdBy, isAdmin, err := h.svc.ArticleEditAccess(r.Context(), title, session.UserID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Article not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error checking permissions", http.StatusInternalServerError)
		return
	}
	if createdBy != session.UserID && !isAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	filePaths, err := h.svc.DeleteArticle(r.Context(), title)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Article not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error deleting article", http.StatusInternalServerError)
		}
		return
	}

	for _, fp := range filePaths {
		if removeErr := os.Remove(h.resolveAppPath(fp)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			log.Printf("wiki: could not remove article attachment file %s: %v", fp, removeErr)
		}
	}

	http.Redirect(w, r, "/wiki", http.StatusSeeOther)
}

func (h *Handlers) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 || parts[3] == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	articleID := parts[3]

	createdBy, isAdmin, authErr := h.svc.ArticleUploadAccess(r.Context(), articleID, session.UserID)
	if authErr != nil {
		if errors.Is(authErr, web.ErrNotFound) {
			http.Error(w, "Article not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Error checking permissions", http.StatusInternalServerError)
		return
	}
	if createdBy != session.UserID && !isAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	const maxUploadBytes = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
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
			log.Printf("wiki: error saving %s for article %s: %v", fh.Filename, articleID, saveErr)
			errMsgs = append(errMsgs, fmt.Sprintf("could not save %s", fh.Filename))
			continue
		}
		if dbErr := h.svc.AddAttachment(r.Context(), articleID, fp, sanitizeFilename(fh.Filename), session.UserID); dbErr != nil {
			log.Printf("wiki: error storing attachment for article %s: %v", articleID, dbErr)
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
		articleTitle, titErr := h.svc.GetArticleTitle(r.Context(), articleID)
		if titErr != nil {
			http.Error(w, "Error loading article", http.StatusInternalServerError)
			return
		}
		article, loadErr := h.svc.GetArticle(r.Context(), articleTitle)
		if loadErr != nil {
			http.Error(w, "Error loading article", http.StatusInternalServerError)
			return
		}
		data := &WikiViewPageData{
			BasePageData: BasePageData{Username: session.Username},
			Article:      article,
			Flash:        flash,
			Error:        errMsg,
		}
		if err := h.renderer.RenderSection(w, "wiki_view", "wiki_attachments_section", data); err != nil {
			http.Error(w, "Error rendering attachments", http.StatusInternalServerError)
		}
		return
	}

	redirect := "/wiki"
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Path != "" && !strings.HasPrefix(u.Path, "//") {
			redirect = u.Path
		}
	}
	if errMsg != "" {
		redirect += "?error=" + url.QueryEscape(errMsg)
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handlers) deleteAttachment(w http.ResponseWriter, r *http.Request, attachmentID string) {
	session := auth.MustSessionFromContext(r.Context())

	createdBy, isAdmin, authErr := h.svc.AttachmentDeleteAccess(r.Context(), attachmentID, session.UserID)
	if authErr != nil {
		if errors.Is(authErr, web.ErrNotFound) {
			http.Error(w, "Attachment not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error checking permissions", http.StatusInternalServerError)
		}
		return
	}
	if createdBy != session.UserID && !isAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	articleTitle, filePath, err := h.svc.DeleteAttachment(r.Context(), attachmentID)
	if err != nil {
		log.Printf("wiki: failed to delete attachment %s: %v", attachmentID, err)
		http.Error(w, "Error deleting attachment", http.StatusInternalServerError)
		return
	}

	if removeErr := os.Remove(h.resolveAppPath(filePath)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		log.Printf("wiki: could not remove attachment file %s: %v", filePath, removeErr)
	}

	redirect := ""
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && u.Path != "" && !strings.HasPrefix(u.Path, "//") {
			redirect = u.Path
		}
	}
	if redirect == "" {
		if articleTitle != "" {
			redirect = "/wiki/view/" + url.PathEscape(articleTitle)
		} else {
			redirect = "/wiki"
		}
	}

	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handlers) downloadAttachment(w http.ResponseWriter, r *http.Request, attachmentID string) {
	filePath, original, err := h.svc.GetAttachmentInfo(r.Context(), attachmentID)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	resolved := h.resolveAppPath(filePath)
	if resolved == "" {
		http.NotFound(w, r)
		return
	}
	if original != "" && !isImage(mime.TypeByExtension(filepath.Ext(original))) {
		setDownloadHeaders(w, original)
	}
	http.ServeFile(w, r, resolved)
}

// Preview renders Markdown to HTML for the live editor preview pane.
// POST /wiki/preview — accepts {"content": "..."} JSON or form field "content".
func (h *Handlers) Preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var content string
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Content string `json:"content"`
		}
		if err := decodeJSON(r, &body); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		content = body.Content
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}
		content = r.FormValue("content")
	}

	rendered := web.RenderMarkdown(content)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(string(rendered))) //nolint:errcheck
}
