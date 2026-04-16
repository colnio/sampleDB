package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"sampleDB/internal/auth"
)

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: writeJSON encode error: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

type apiArticleItem struct {
	ID             int       `json:"id"`
	Title          string    `json:"title"`
	CreatedAt      time.Time `json:"created_at"`
	LastModifiedAt time.Time `json:"last_modified_at"`
}

type apiArticleDetail struct {
	ID             int                    `json:"id"`
	Title          string                 `json:"title"`
	Content        string                 `json:"content"`
	CreatedAt      time.Time              `json:"created_at"`
	LastModifiedAt time.Time              `json:"last_modified_at"`
	Attachments    []apiArticleAttachment `json:"attachments"`
}

type apiArticleAttachment struct {
	ID           int       `json:"id"`
	OriginalName string    `json:"original_name"`
	URL          string    `json:"url"`
	IsImage      bool      `json:"is_image"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

type apiUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	GroupName string `json:"group"`
}

// ---------------------------------------------------------------------------
// Pagination helper
// ---------------------------------------------------------------------------

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 100
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}

// ---------------------------------------------------------------------------
// /api/v1/wiki — dispatcher
// ---------------------------------------------------------------------------

func apiWikiHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/v1/wiki"), "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			apiListArticles(w, r)
		case http.MethodPost:
			apiCreateArticle(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	articleID := strings.TrimPrefix(path, "/")
	switch r.Method {
	case http.MethodGet:
		apiGetArticle(w, r, articleID)
	case http.MethodPut:
		apiUpdateArticle(w, r, articleID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func apiListArticles(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	var total int
	if err := theApp.DB.QueryRow(r.Context(), "SELECT COUNT(*) FROM articles").Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	rows, err := theApp.DB.Query(r.Context(), `
		SELECT article_id, title, created_at,
		       COALESCE(last_modified_at, created_at)
		  FROM articles
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		log.Printf("api: listArticles error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	items := make([]apiArticleItem, 0)
	for rows.Next() {
		var item apiArticleItem
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.LastModifiedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		log.Printf("api: listArticles rows error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func apiGetArticle(w http.ResponseWriter, r *http.Request, articleID string) {
	id, err := strconv.Atoi(articleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}
	var detail apiArticleDetail
	err = theApp.DB.QueryRow(r.Context(), `
		SELECT article_id, title, content, created_at,
		       COALESCE(last_modified_at, created_at)
		  FROM articles
		 WHERE article_id = $1`, id).Scan(
		&detail.ID, &detail.Title, &detail.Content,
		&detail.CreatedAt, &detail.LastModifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		log.Printf("api: getArticle error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Attachments
	detail.Attachments = make([]apiArticleAttachment, 0)
	attRows, attErr := theApp.DB.Query(r.Context(), `
		SELECT attachment_id, original_name, attachment_address, uploaded_at
		  FROM article_attachments
		 WHERE article_id = $1`, detail.ID)
	if attErr != nil {
		log.Printf("api: getArticle attachments error: %v", attErr)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer attRows.Close()
	for attRows.Next() {
		var att apiArticleAttachment
		var addr string
		if scanErr := attRows.Scan(&att.ID, &att.OriginalName, &addr, &att.UploadedAt); scanErr != nil {
			log.Printf("api: getArticle attachment scan error: %v", scanErr)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		att.URL = fmt.Sprintf("/wiki/attachment/%d", att.ID)
		att.IsImage = isImage(mime.TypeByExtension(filepath.Ext(addr)))
		detail.Attachments = append(detail.Attachments, att)
	}
	if err := attRows.Err(); err != nil {
		log.Printf("api: getArticle attachments iteration error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

type articleRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func apiCreateArticle(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req articleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	var id int
	var createdAt time.Time
	err := theApp.DB.QueryRow(r.Context(), `
		INSERT INTO articles (title, content, created_by, last_modified_by)
		VALUES ($1, $2, $3, $3)
		RETURNING article_id, created_at`,
		req.Title, req.Content, session.UserID,
	).Scan(&id, &createdAt)
	if err != nil {
		log.Printf("api: createArticle error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, apiArticleDetail{
		ID:             id,
		Title:          req.Title,
		Content:        req.Content,
		CreatedAt:      createdAt,
		LastModifiedAt: createdAt,
		Attachments:    []apiArticleAttachment{},
	})
}

func apiUpdateArticle(w http.ResponseWriter, r *http.Request, articleID string) {
	id, err := strconv.Atoi(articleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	// Check ownership: caller must be the article creator or an admin.
	var createdBy int
	var isAdmin bool
	err = theApp.DB.QueryRow(r.Context(),
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.article_id = $2`,
		session.UserID, id).Scan(&createdBy, &isAdmin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		log.Printf("api: updateArticle auth check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if createdBy != session.UserID && !isAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req articleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	tag, err := theApp.DB.Exec(r.Context(), `
		UPDATE articles
		   SET title            = $1,
		       content          = $2,
		       last_modified_at = NOW(),
		       last_modified_by = $3
		 WHERE article_id = $4`,
		req.Title, req.Content, session.UserID, id)
	if err != nil {
		log.Printf("api: updateArticle error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "article not found")
		return
	}

	apiGetArticle(w, r, articleID)
}

// ---------------------------------------------------------------------------
// /api/v1/users
// ---------------------------------------------------------------------------

func apiUsersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	rows, err := theApp.DB.Query(r.Context(), `
		SELECT user_id, username, COALESCE(btrim("group"), '')
		  FROM users
		 WHERE is_approved = true
		   AND COALESCE(deleted, false) = false
		 ORDER BY username`)
	if err != nil {
		log.Printf("api: listUsers error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	users := make([]apiUser, 0)
	for rows.Next() {
		var u apiUser
		if err := rows.Scan(&u.ID, &u.Username, &u.GroupName); err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		log.Printf("api: listUsers rows error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": users,
		"total": len(users),
	})
}

// ---------------------------------------------------------------------------
// /api/v1/equipment
// ---------------------------------------------------------------------------

func apiEquipmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	equip, err := getAllEquipment(r.Context())
	if err != nil {
		log.Printf("api: listEquipment error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": equip,
		"total": len(equip),
	})
}

// apiNotFoundHandler is a catch-all for /api/ paths that don't match any registered route.
// It returns a JSON 404 instead of falling through to the HTML root handler.
func apiNotFoundHandler(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not found")
}
