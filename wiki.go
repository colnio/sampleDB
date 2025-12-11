package main

import (
	"context"
	"errors"
	"html/template"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"sampleDB/internal/auth"
)

type Article struct {
	ID             int
	Title          string
	Content        ArticleContent
	CreatedBy      int
	CreatedAt      time.Time
	LastModifiedAt time.Time
	LastModifiedBy int
	Attachments    []ArticleAttachment
}

type ArticleAttachment struct {
	IsImage      bool
	ID           int
	ArticleID    int
	Address      string
	OriginalName string
	UploadedAt   time.Time
	UploadedBy   int
}

type WikiPageData struct {
	Username       string
	Articles       []Article
	Article        *Article
	Error          string
	Success        string
	IsAdmin        bool
	EditingContent bool
	IsPartial      bool
}

type ArticleContent struct {
	Raw  string
	HTML template.HTML
}

func handleWiki(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/wiki" {
		listArticlesHandler(w, r)
		return
	}

	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/wiki/"), "/")
	if len(pathParts) < 1 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	switch pathParts[0] {
	case "new":
		newArticleHandler(w, r)
	case "edit":
		editArticleHandler(w, r)
	case "delete":
		deleteArticleHandler(w, r)
	case "upload":
		uploadArticleAttachmentHandler(w, r)
	case "view":
		if len(pathParts) < 2 || pathParts[1] == "" {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		viewArticleHandler(w, r, pathParts[1])
	default:
		// Backward compatibility: /wiki/{title} -> /wiki/view/{title}
		http.Redirect(w, r, "/wiki/view/"+pathParts[0], http.StatusSeeOther)
	}
}

func listArticlesHandler(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	rows, err := dbPool.Query(context.Background(),
		`SELECT article_id, title, created_at FROM articles ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, "Error retrieving articles", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var article Article
		err := rows.Scan(&article.ID, &article.Title, &article.CreatedAt)
		if err != nil {
			continue
		}
		articles = append(articles, article)
	}

	data := struct {
		BasePageData
		Articles []Article
	}{
		BasePageData: BasePageData{Username: session.Username, IsAdmin: false},
		Articles:     articles,
	}
	row := dbPool.QueryRow(context.Background(), `SELECT admin
	FROM users
	WHERE username = $1`, session.Username)

	if err := row.Scan(&data.BasePageData.IsAdmin); err != nil {
		log.Printf("wiki: unable to load admin flag for %s: %v", session.Username, err)
	}

	tmpl, err := parseTemplates("templates/wiki_list.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "base", data)
}

func loadArticleData(ctx context.Context, session auth.Session, title string) (*Article, error) {
	var article Article
	var rawContent string
	err := dbPool.QueryRow(ctx,
		`SELECT article_id, title, content, created_at, created_by,
		        COALESCE(last_modified_at, created_at) AS last_modified_at,
		        COALESCE(last_modified_by, created_by) AS last_modified_by
		 FROM articles WHERE title = $1`, title).Scan(
		&article.ID, &article.Title, &rawContent, &article.CreatedAt, &article.CreatedBy,
		&article.LastModifiedAt, &article.LastModifiedBy)
	if err != nil {
		return nil, err
	}

	// Set both raw content and rendered HTML
	article.Content = ArticleContent{
		Raw:  rawContent,
		HTML: renderMarkdown(rawContent),
	}

	// Get attachments
	rows, err := dbPool.Query(ctx,
		`SELECT attachment_id, original_name, attachment_address, uploaded_at 
         FROM article_attachments WHERE article_id = $1`, article.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var att ArticleAttachment
			err := rows.Scan(&att.ID, &att.OriginalName, &att.Address, &att.UploadedAt)
			if err == nil {
				att.IsImage = isImage(mime.TypeByExtension(filepath.Ext(att.Address)))
				article.Attachments = append(article.Attachments, att)
			}
		}
	}

	return &article, nil
}

func viewArticleHandler(w http.ResponseWriter, r *http.Request, title string) {
	session := auth.MustSessionFromContext(r.Context())

	// Check if this is an HTMX request specifically targeting the content panel
	// (not a full page load via hx-boost)
	if isHTMXRequest(r) && r.Method == http.MethodGet {
		target := r.Header.Get("HX-Target")
		// Only render just the panel if explicitly targeting it
		// HTMX boost will target "page-root" for full page loads
		if target == "article-content-panel" {
			renderArticleContentPanel(w, r, session, title, "", "", false)
			return
		}
		// For hx-boost requests targeting page-root, we need to render the full page
		// So we continue to the full page rendering below
	}

	article, err := loadArticleData(r.Context(), session, title)
	if err != nil {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	isAdmin := false
	if err := dbPool.QueryRow(r.Context(),
		"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		session.UserID).Scan(&isAdmin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Redirect(w, r, "/logout", http.StatusSeeOther)
			return
		}
		log.Printf("wiki: unable to load admin status for user %d: %v", session.UserID, err)
		isAdmin = false
	}

	data := struct {
		BasePageData
		Article        *Article
		EditingContent bool
		Flash          string
		Error          string
	}{
		BasePageData:   BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Article:        article,
		EditingContent: false,
	}

	tmpl, err := parseTemplates("templates/wiki_view.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("wiki: error rendering view template: %v", err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

func renderArticleContentPanel(w http.ResponseWriter, r *http.Request, session auth.Session, title, flash, errMsg string, editing bool) {
	article, err := loadArticleData(r.Context(), session, title)
	if err != nil {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	isAdmin := false
	if err := dbPool.QueryRow(r.Context(),
		"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		session.UserID).Scan(&isAdmin); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("wiki: unable to load admin status for user %d: %v", session.UserID, err)
		}
	}

	data := struct {
		BasePageData
		Article        *Article
		EditingContent bool
		IsPartial      bool
		Flash          string
		Error          string
	}{
		BasePageData:   BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Article:        article,
		EditingContent: editing,
		IsPartial:      true,
		Flash:          flash,
		Error:          errMsg,
	}

	if err := renderTemplateSection(w, "templates/wiki_view.html", "article_content_panel", data); err != nil {
		http.Error(w, "Error rendering article content", http.StatusInternalServerError)
	}
}

func newArticleHandler(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	if r.Method == http.MethodGet {
		data := struct {
			BasePageData
			Article *Article // nil for new article
		}{
			BasePageData: BasePageData{Username: session.Username},
		}

		tmpl, err := parseTemplates("templates/wiki_edit.html")
		if err != nil {
			http.Error(w, "Error loading template", http.StatusInternalServerError)
			return
		}

		tmpl.ExecuteTemplate(w, "base", data)
		return
	}

	// Handle POST request
	title := r.FormValue("title")
	content := r.FormValue("content")

	_, err := dbPool.Exec(context.Background(),
		`INSERT INTO articles (title, content, created_by, last_modified_by) 
         VALUES ($1, $2, $3, $3)`,
		title, content, session.UserID)
	if err != nil {
		log.Printf("wiki: error creating article %q: %v", title, err)
		http.Error(w, "Error creating article", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/wiki/view/"+title, http.StatusSeeOther)
}

func editArticleHandler(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/wiki/edit/"), "/")
	if len(pathParts) < 1 || pathParts[0] == "" {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	title := pathParts[0]

	if r.Method == http.MethodGet {
		// Check if this is an HTMX request for inline editing
		if isHTMXRequest(r) {
			renderArticleContentPanel(w, r, session, title, "", "", true)
			return
		}

		var article Article
		var rawContent string
		err := dbPool.QueryRow(context.Background(),
			`SELECT article_id, title, content FROM articles WHERE title = $1`,
			title).Scan(&article.ID, &article.Title, &rawContent)
		if err != nil {
			http.Error(w, "Article not found", http.StatusNotFound)
			return
		}

		article.Content = ArticleContent{Raw: rawContent}

		data := struct {
			BasePageData
			Article *Article
		}{
			BasePageData: BasePageData{Username: session.Username},
			Article:      &article,
		}

		tmpl, err := parseTemplates("templates/wiki_edit.html")
		if err != nil {
			http.Error(w, "Error loading template", http.StatusInternalServerError)
			return
		}

		tmpl.ExecuteTemplate(w, "base", data)
		return
	}

	// Handle POST request
	if err := r.ParseForm(); err != nil {
		if isHTMXRequest(r) {
			renderArticleContentPanel(w, r, session, title, "", "Invalid form submission.", true)
			return
		}
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	content := r.FormValue("content")
	_, err := dbPool.Exec(context.Background(),
		`UPDATE articles SET content = $1, last_modified_at = NOW(), 
         last_modified_by = $2 WHERE title = $3`,
		content, session.UserID, title)
	if err != nil {
		if isHTMXRequest(r) {
			renderArticleContentPanel(w, r, session, title, "", "Failed to update article content.", true)
			return
		}
		http.Error(w, "Error updating article", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		renderArticleContentPanel(w, r, session, title, "Content updated", "", false)
		return
	}

	http.Redirect(w, r, "/wiki/view/"+title, http.StatusSeeOther)
}

func deleteArticleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	title := parts[3]

	_, err := dbPool.Exec(context.Background(),
		"DELETE FROM articles WHERE title = $1", title)
	if err != nil {
		http.Error(w, "Error deleting article", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/wiki", http.StatusSeeOther)
}

func uploadArticleAttachmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	articleID := parts[3]

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

	filepath, err := saveUploadedFile(file, header.Filename)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	_, err = dbPool.Exec(context.Background(),
		`INSERT INTO article_attachments 
         (article_id, attachment_address, original_name, uploaded_by) 
         VALUES ($1, $2, $3, $4)`,
		articleID, filepath, header.Filename, session.UserID)
	if err != nil {
		log.Printf("wiki: error storing attachment metadata for article %s: %v", articleID, err)
		http.Error(w, "Error storing attachment info", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
}

func handleAttachmentWiki(w http.ResponseWriter, r *http.Request) {
	trimmedPath := strings.TrimSuffix(r.URL.Path, "/")
	pathParts := strings.Split(trimmedPath, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	attachmentID := pathParts[3]

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(trimmedPath, "/delete"):
		articleTitle, err := deleteAttachmentWiki(attachmentID)
		if err != nil {
			log.Printf("wiki: failed to delete attachment %s: %v", attachmentID, err)
			http.Error(w, "Error deleting attachment", http.StatusInternalServerError)
			return
		}

		redirect := r.Header.Get("Referer")
		if redirect == "" {
			if articleTitle != "" {
				redirect = "/wiki/view/" + url.PathEscape(articleTitle)
			} else {
				redirect = "/wiki"
			}
		}

		http.Redirect(w, r, redirect, http.StatusSeeOther)
		return
	case r.Method == http.MethodGet:
		downloadAttachmentHandlerWiki(w, r, attachmentID)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func deleteAttachmentWiki(attachmentID string) (string, error) {
	var filepath string
	var articleTitle string
	err := dbPool.QueryRow(context.Background(),
		`SELECT aa.attachment_address, a.title
         FROM article_attachments aa
         JOIN articles a ON aa.article_id = a.article_id
         WHERE aa.attachment_id = $1`,
		attachmentID).Scan(&filepath, &articleTitle)
	if err != nil {
		return "", err
	}

	// Delete from database
	if _, err = dbPool.Exec(context.Background(),
		"DELETE FROM article_attachments WHERE attachment_id = $1",
		attachmentID); err != nil {
		return "", err
	}

	// Delete file from filesystem
	if removeErr := os.Remove(resolveAppPath(filepath)); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return "", removeErr
	}

	return articleTitle, nil
}

func downloadAttachmentHandlerWiki(w http.ResponseWriter, r *http.Request, attachmentID string) {
	var (
		filepath string
		original string
	)
	err := dbPool.QueryRow(context.Background(),
		"SELECT attachment_address, original_name FROM article_attachments WHERE attachment_id = $1",
		attachmentID).Scan(&filepath, &original)
	if err != nil {
		http.Error(w, "Attachment not found", http.StatusNotFound)
		return
	}

	if original != "" {
		setDownloadHeaders(w, original)
	}

	// Serve the file
	http.ServeFile(w, r, resolveAppPath(filepath))
}
