package main

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

type Article struct {
	ID             int
	Title          string
	Content        string
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
	Username string
	Articles []Article
	Article  *Article
	Error    string
	Success  string
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
	default:
		viewArticleHandler(w, r)
	}
}

func listArticlesHandler(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)

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
		BasePageData: BasePageData{Username: session.Username},
		Articles:     articles,
	}

	tmpl, err := parseTemplates("templates/wiki_list.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "base", data)
}

func viewArticleHandler(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)
	title := strings.TrimPrefix(r.URL.Path, "/wiki/")

	var article Article
	err := dbPool.QueryRow(context.Background(),
		`SELECT article_id, title, content, created_at, created_by 
         FROM articles WHERE title = $1`, title).Scan(
		&article.ID, &article.Title, &article.Content, &article.CreatedAt, &article.CreatedBy)
	if err != nil {
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// Get attachments
	rows, err := dbPool.Query(context.Background(),
		`SELECT attachment_id, original_name, attachment_address, uploaded_at 
         FROM article_attachments WHERE article_id = $1`, article.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var att ArticleAttachment
			err := rows.Scan(&att.ID, &att.OriginalName, &att.Address, &att.UploadedAt)
			if err == nil {
				contentType := mime.TypeByExtension(filepath.Ext(att.Address))
				att.IsImage = isImage(contentType)
				att.Address = filepath.Base(att.Address)
				article.Attachments = append(article.Attachments, att)
			}
		}
	}

	data := struct {
		BasePageData
		Article *Article
	}{
		BasePageData: BasePageData{Username: session.Username},
		Article:      &article,
	}

	tmpl, err := parseTemplates("templates/wiki_view.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "base", data)
}
func newArticleHandler(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)

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
		http.Error(w, "Error creating article", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/wiki/"+title, http.StatusSeeOther)
}

func editArticleHandler(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	fmt.Println(r.URL.Path)
	title := parts[3]
	fmt.Println("Article to edit title: " + title)
	if r.Method == http.MethodGet {
		var article Article
		err := dbPool.QueryRow(context.Background(),
			`SELECT article_id, title, content FROM articles WHERE title = $1`,
			title).Scan(&article.ID, &article.Title, &article.Content)
		if err != nil {
			http.Error(w, "Article not found", http.StatusNotFound)
			return
		}

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
	content := r.FormValue("content")
	_, err := dbPool.Exec(context.Background(),
		`UPDATE articles SET content = $1, last_modified_at = NOW(), 
         last_modified_by = $2 WHERE title = $3`,
		content, session.UserID, title)
	if err != nil {
		http.Error(w, "Error updating article", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/wiki/"+title, http.StatusSeeOther)
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

	session := r.Context().Value("user").(Session)
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
		fmt.Println(err)
		http.Error(w, "Error storing attachment info", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, r.Header.Get("Referer"), http.StatusSeeOther)
}
