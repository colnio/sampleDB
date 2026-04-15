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

type apiSampleItem struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Keywords        string    `json:"keywords"`
	Owner           string    `json:"owner"`
	OwnerUserID     *int      `json:"owner_user_id,omitempty"`
	OwnerUsername   *string   `json:"owner_username,omitempty"`
	AttachmentCount int       `json:"attachment_count"`
	CreatedAt       time.Time `json:"created_at"`
}

type apiSampleDetail struct {
	ID              int                  `json:"id"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Keywords        string               `json:"keywords"`
	Owner           string               `json:"owner"`
	OwnerUserID     *int                 `json:"owner_user_id,omitempty"`
	OwnerUsername   *string              `json:"owner_username,omitempty"`
	SamplePrep      string               `json:"sample_prep"`
	AttachmentCount int                  `json:"attachment_count"`
	CreatedAt       time.Time            `json:"created_at"`
	Steps           []apiFabricationStep `json:"fabrication_steps"`
	Attachments     []apiAttachment      `json:"attachments"`
}

type apiFabricationStep struct {
	ID             int       `json:"id"`
	SampleID       int       `json:"sample_id"`
	StepOrder      int       `json:"step_order"`
	StepDate       *string   `json:"step_date,omitempty"`
	PersonUserID   *int      `json:"person_user_id,omitempty"`
	PersonUsername *string   `json:"person_username,omitempty"`
	Description    string    `json:"description"`
	EquipmentID    *int      `json:"equipment_id,omitempty"`
	EquipmentName  *string   `json:"equipment_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type apiAttachment struct {
	ID           int       `json:"id"`
	OriginalName string    `json:"original_name"`
	URL          string    `json:"url"`
	ContentType  string    `json:"content_type"`
	IsImage      bool      `json:"is_image"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

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
// Conversion helpers
// ---------------------------------------------------------------------------

func fabricationStepToAPI(s FabricationStep) apiFabricationStep {
	item := apiFabricationStep{
		ID:             s.ID,
		SampleID:       s.SampleID,
		StepOrder:      s.StepOrder,
		PersonUserID:   s.PersonUserID,
		PersonUsername: s.PersonUsername,
		Description:    s.Description,
		EquipmentID:    s.EquipmentID,
		EquipmentName:  s.EquipmentName,
		CreatedAt:      s.CreatedAt,
	}
	if s.StepDate != nil {
		d := s.StepDate.Format("2006-01-02")
		item.StepDate = &d
	}
	return item
}

func attachmentToAPI(att Attachment) apiAttachment {
	return apiAttachment{
		ID:           att.ID,
		OriginalName: att.OriginalName,
		URL:          fmt.Sprintf("/attachment/%d", att.ID),
		ContentType:  att.ContentType,
		IsImage:      att.IsImage,
		UploadedAt:   att.UploadedAt,
	}
}

// ---------------------------------------------------------------------------
// /api/v1/samples — dispatcher
// ---------------------------------------------------------------------------

// apiSamplesHandler routes requests under /api/v1/samples[/...].
func apiSamplesHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/samples")
	path = strings.TrimRight(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			apiListSamples(w, r)
		case http.MethodPost:
			apiCreateSample(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// /api/v1/samples/{id}[/steps]
	parts := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)
	sampleID := parts[0]
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch sub {
	case "":
		switch r.Method {
		case http.MethodGet:
			apiGetSample(w, r, sampleID)
		case http.MethodPut:
			apiUpdateSample(w, r, sampleID)
		case http.MethodDelete:
			apiDeleteSample(w, r, sampleID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "steps":
		switch r.Method {
		case http.MethodGet:
			apiListSteps(w, r, sampleID)
		case http.MethodPost:
			apiAddStep(w, r, sampleID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// ---------------------------------------------------------------------------
// Sample CRUD
// ---------------------------------------------------------------------------

func apiListSamples(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)
	q := r.URL.Query().Get("q")
	ownerFilter := r.URL.Query().Get("owner")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	var conditions []string
	var args []any

	if q != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf(
			"(s.sample_name ILIKE $%[1]d OR s.sample_owner ILIKE $%[1]d OR s.sample_keywords ILIKE $%[1]d)", idx))
		args = append(args, "%"+q+"%")
	}
	if ownerFilter != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("s.sample_owner ILIKE $%d", idx))
		args = append(args, "%"+ownerFilter+"%")
	}
	if from != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("s.created_at >= $%d", idx))
		args = append(args, from)
	}
	if to != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("s.created_at <= $%d", idx))
		args = append(args, to+" 23:59:59")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Total count
	countQuery := "SELECT COUNT(*) FROM samples s" + whereClause
	var total int
	if err := dbPool.QueryRow(r.Context(), countQuery, args...).Scan(&total); err != nil {
		log.Printf("api: listSamples count error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Paginated results with attachment count
	pagedArgs := make([]any, len(args), len(args)+2)
	copy(pagedArgs, args)
	pagedArgs = append(pagedArgs, limit, offset)
	mainQuery := `
		SELECT s.sample_id,
		       s.sample_name,
		       COALESCE(s.sample_description, ''),
		       COALESCE(s.sample_keywords, ''),
		       COALESCE(s.sample_owner, ''),
		       s.owner_user_id,
		       u.username,
		       COUNT(a.attachment_id) AS attachment_count,
		       s.created_at
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		  LEFT JOIN attachments a ON a.source_id = s.sample_id AND a.source_type = 'sample'` +
		whereClause +
		fmt.Sprintf(` GROUP BY s.sample_id, u.username
		 ORDER BY s.sample_name
		 LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

	rows, err := dbPool.Query(r.Context(), mainQuery, pagedArgs...)
	if err != nil {
		log.Printf("api: listSamples query error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	defer rows.Close()

	items := make([]apiSampleItem, 0)
	for rows.Next() {
		var item apiSampleItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Keywords, &item.Owner,
			&item.OwnerUserID, &item.OwnerUsername,
			&item.AttachmentCount, &item.CreatedAt,
		); err != nil {
			log.Printf("api: listSamples scan error: %v", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
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

func apiGetSample(w http.ResponseWriter, r *http.Request, sampleID string) {
	var detail apiSampleDetail
	var samplePrep string

	err := dbPool.QueryRow(r.Context(), `
		SELECT s.sample_id,
		       s.sample_name,
		       COALESCE(s.sample_description, ''),
		       COALESCE(s.sample_keywords, ''),
		       COALESCE(s.sample_owner, ''),
		       s.owner_user_id,
		       u.username,
		       COALESCE(s.sample_prep, ''),
		       s.created_at
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		 WHERE s.sample_id = $1`, sampleID).Scan(
		&detail.ID, &detail.Name, &detail.Description, &detail.Keywords, &detail.Owner,
		&detail.OwnerUserID, &detail.OwnerUsername,
		&samplePrep, &detail.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "sample not found")
			return
		}
		log.Printf("api: getSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	detail.SamplePrep = samplePrep

	// Fabrication steps
	if idInt, err := strconv.Atoi(sampleID); err == nil {
		steps, stepsErr := getFabricationSteps(idInt)
		if stepsErr != nil {
			log.Printf("api: getSample steps error: %v", stepsErr)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		detail.Steps = make([]apiFabricationStep, 0, len(steps))
		for _, s := range steps {
			detail.Steps = append(detail.Steps, fabricationStepToAPI(s))
		}
	} else {
		detail.Steps = []apiFabricationStep{}
	}

	// Attachments
	atts, attsErr := getAttachments(sampleID)
	if attsErr != nil {
		log.Printf("api: getSample attachments error: %v", attsErr)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	detail.AttachmentCount = len(atts)
	detail.Attachments = make([]apiAttachment, 0, len(atts))
	for _, a := range atts {
		detail.Attachments = append(detail.Attachments, attachmentToAPI(a))
	}

	writeJSON(w, http.StatusOK, detail)
}

type sampleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Keywords    string `json:"keywords"`
	Owner       string `json:"owner"`
	SamplePrep  string `json:"sample_prep"`
}

func apiCreateSample(w http.ResponseWriter, r *http.Request) {
	var req sampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	var id int
	var createdAt time.Time
	err := dbPool.QueryRow(r.Context(), `
		INSERT INTO samples (sample_name, sample_description, sample_keywords, sample_owner, sample_prep)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING sample_id, created_at`,
		req.Name, req.Description, req.Keywords, req.Owner, req.SamplePrep,
	).Scan(&id, &createdAt)
	if err != nil {
		log.Printf("api: createSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusCreated, apiSampleItem{
		ID:              id,
		Name:            req.Name,
		Description:     req.Description,
		Keywords:        req.Keywords,
		Owner:           req.Owner,
		AttachmentCount: 0,
		CreatedAt:       createdAt,
	})
}

func apiUpdateSample(w http.ResponseWriter, r *http.Request, sampleID string) {
	var req sampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	tag, err := dbPool.Exec(r.Context(), `
		UPDATE samples
		   SET sample_name        = $1,
		       sample_description = $2,
		       sample_keywords    = $3,
		       sample_owner       = $4,
		       sample_prep        = $5
		 WHERE sample_id = $6`,
		req.Name, req.Description, req.Keywords, req.Owner, req.SamplePrep, sampleID)
	if err != nil {
		log.Printf("api: updateSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "sample not found")
		return
	}

	apiGetSample(w, r, sampleID)
}

func apiDeleteSample(w http.ResponseWriter, r *http.Request, sampleID string) {
	tag, err := dbPool.Exec(r.Context(), `DELETE FROM samples WHERE sample_id = $1`, sampleID)
	if err != nil {
		log.Printf("api: deleteSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "sample not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Fabrication steps API
// ---------------------------------------------------------------------------

func apiListSteps(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	steps, err := getFabricationSteps(id)
	if err != nil {
		log.Printf("api: listSteps error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	items := make([]apiFabricationStep, 0, len(steps))
	for _, s := range steps {
		items = append(items, fabricationStepToAPI(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  len(items),
		"limit":  len(items),
		"offset": 0,
	})
}

type stepRequest struct {
	StepOrder    int    `json:"step_order"`
	StepDate     string `json:"step_date"`
	PersonUserID *int   `json:"person_user_id"`
	Description  string `json:"description"`
	EquipmentID  *int   `json:"equipment_id"`
}

func stepRequestToStep(req stepRequest) FabricationStep {
	step := FabricationStep{
		StepOrder:    req.StepOrder,
		PersonUserID: req.PersonUserID,
		Description:  req.Description,
		EquipmentID:  req.EquipmentID,
	}
	if req.StepDate != "" {
		if t, err := time.Parse("2006-01-02", req.StepDate); err == nil {
			step.StepDate = &t
		}
	}
	return step
}

func apiAddStep(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	var req stepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	step := stepRequestToStep(req)

	created, err := addFabricationStep(id, step)
	if err != nil {
		log.Printf("api: addStep error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, fabricationStepToAPI(created))
}

// apiStepsHandler routes requests under /api/v1/steps/{id}.
func apiStepsHandler(w http.ResponseWriter, r *http.Request) {
	stepID := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/v1/steps/"), "/")
	if stepID == "" {
		writeError(w, http.StatusBadRequest, "step id required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		apiUpdateStep(w, r, stepID)
	case http.MethodDelete:
		apiDeleteStep(w, r, stepID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func apiUpdateStep(w http.ResponseWriter, r *http.Request, stepID string) {
	id, err := strconv.Atoi(stepID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}

	var req stepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	step := stepRequestToStep(req)

	if err := updateFabricationStep(id, step); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		log.Printf("api: updateStep error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func apiDeleteStep(w http.ResponseWriter, r *http.Request, stepID string) {
	id, err := strconv.Atoi(stepID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}

	if err := deleteFabricationStep(id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		log.Printf("api: deleteStep error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if err := dbPool.QueryRow(r.Context(), "SELECT COUNT(*) FROM articles").Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	rows, err := dbPool.Query(r.Context(), `
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
	var detail apiArticleDetail
	err := dbPool.QueryRow(r.Context(), `
		SELECT article_id, title, content, created_at,
		       COALESCE(last_modified_at, created_at)
		  FROM articles
		 WHERE article_id = $1`, articleID).Scan(
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
	attRows, err := dbPool.Query(r.Context(), `
		SELECT attachment_id, original_name, attachment_address, uploaded_at
		  FROM article_attachments
		 WHERE article_id = $1`, detail.ID)
	if err == nil {
		defer attRows.Close()
		for attRows.Next() {
			var att apiArticleAttachment
			var addr string
			if scanErr := attRows.Scan(&att.ID, &att.OriginalName, &addr, &att.UploadedAt); scanErr == nil {
				att.URL = fmt.Sprintf("/wiki/attachment/%d", att.ID)
				att.IsImage = isImage(mime.TypeByExtension(filepath.Ext(addr)))
				detail.Attachments = append(detail.Attachments, att)
			}
		}
	}

	writeJSON(w, http.StatusOK, detail)
}

type articleRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func apiCreateArticle(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

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
	err := dbPool.QueryRow(r.Context(), `
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
	session := auth.MustSessionFromContext(r.Context())

	var req articleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	tag, err := dbPool.Exec(r.Context(), `
		UPDATE articles
		   SET title            = $1,
		       content          = $2,
		       last_modified_at = NOW(),
		       last_modified_by = $3
		 WHERE article_id = $4`,
		req.Title, req.Content, session.UserID, articleID)
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

	rows, err := dbPool.Query(r.Context(), `
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
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  users,
		"total":  len(users),
		"limit":  len(users),
		"offset": 0,
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

	equip, err := getAllEquipment()
	if err != nil {
		log.Printf("api: listEquipment error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  equip,
		"total":  len(equip),
		"limit":  len(equip),
		"offset": 0,
	})
}

