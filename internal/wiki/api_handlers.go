package wiki

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"sampleDB/internal/activity"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	web.JSON(w, status, v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	web.JSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	return web.DecodeJSON(r, v)
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
// /api/v1/wiki dispatcher
// ---------------------------------------------------------------------------

// APIWiki handles /api/v1/wiki[/...] requests.
func (h *Handlers) APIWiki(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/v1/wiki"), "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.apiListArticles(w, r)
		case http.MethodPost:
			h.apiCreateArticle(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	articleID := strings.TrimPrefix(path, "/")
	switch r.Method {
	case http.MethodGet:
		h.apiGetArticle(w, r, articleID)
	case http.MethodPut:
		h.apiUpdateArticle(w, r, articleID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// API handlers
// ---------------------------------------------------------------------------

func (h *Handlers) apiListArticles(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	items, total, err := h.svc.APIListArticles(r.Context(), limit, offset)
	if err != nil {
		log.Printf("wiki api: listArticles error: %v", err)
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

func (h *Handlers) apiGetArticle(w http.ResponseWriter, r *http.Request, articleID string) {
	id, err := strconv.Atoi(articleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}

	detail, err := h.svc.APIGetArticle(r.Context(), id)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		log.Printf("wiki api: getArticle error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (h *Handlers) apiCreateArticle(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req ArticleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	detail, err := h.svc.APICreateArticle(r.Context(), req.Title, req.Content, session.UserID)
	if err != nil {
		log.Printf("wiki api: createArticle error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("created"), activity.TargetType("article"), detail.ID, detail.Title)

	writeJSON(w, http.StatusCreated, detail)
}

func (h *Handlers) apiUpdateArticle(w http.ResponseWriter, r *http.Request, articleID string) {
	id, err := strconv.Atoi(articleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid article id")
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	createdBy, isAdmin, err := h.svc.APIArticleEditAccess(r.Context(), id, session.UserID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "article not found")
			return
		}
		log.Printf("wiki api: updateArticle auth check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if createdBy != session.UserID && !isAdmin {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req ArticleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	n, err := h.svc.APIUpdateArticle(r.Context(), id, req.Title, req.Content, session.UserID)
	if err != nil {
		log.Printf("wiki api: updateArticle error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "article not found")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("updated"), activity.TargetType("article"), id, req.Title)

	h.apiGetArticle(w, r, articleID)
}
