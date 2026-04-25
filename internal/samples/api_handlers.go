package samples

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/activity"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	web.JSON(w, status, v)
}

// writeError writes a JSON error body with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	web.JSON(w, status, map[string]string{"error": msg})
}

// writeOwnerError maps CheckOwner sentinel errors to JSON HTTP responses.
func writeOwnerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, web.ErrNotFound):
		writeError(w, http.StatusNotFound, "sample not found")
	case errors.Is(err, web.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden")
	default:
		writeError(w, http.StatusInternalServerError, "database error")
	}
}

// APISamples handles /api/v1/samples[/...].
func (h *Handlers) APISamples(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/samples")
	path = strings.TrimRight(path, "/")

	if path == "" {
		switch r.Method {
		case http.MethodGet:
			h.apiListSamples(w, r)
		case http.MethodPost:
			h.apiCreateSample(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

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
			h.apiGetSample(w, r, sampleID)
		case http.MethodPut:
			h.apiUpdateSample(w, r, sampleID)
		case http.MethodDelete:
			h.apiDeleteSample(w, r, sampleID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "steps":
		switch r.Method {
		case http.MethodGet:
			h.apiListSteps(w, r, sampleID)
		case http.MethodPost:
			h.apiAddStep(w, r, sampleID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case "attachments":
		switch r.Method {
		case http.MethodGet:
			h.apiListAttachments(w, r, sampleID)
		case http.MethodPost:
			h.apiUploadAttachments(w, r, sampleID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// APIAttachments handles /api/v1/attachments/{id}.
//
//	GET    — serve the file bytes (same as /attachment/{id}, but with JSON errors)
//	DELETE — delete the attachment (owner/admin only)
func (h *Handlers) APIAttachments(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/v1/attachments/"), "/")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "attachment id required")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid attachment id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.apiDownloadAttachment(w, r, id)
	case http.MethodDelete:
		h.apiDeleteAttachment(w, r, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// APISteps handles /api/v1/steps/{id}.
func (h *Handlers) APISteps(w http.ResponseWriter, r *http.Request) {
	stepID := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/v1/steps/"), "/")
	if stepID == "" {
		writeError(w, http.StatusBadRequest, "step id required")
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.apiUpdateStep(w, r, stepID)
	case http.MethodDelete:
		h.apiDeleteStep(w, r, stepID)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// ---------------------------------------------------------------------------
// Sample CRUD
// ---------------------------------------------------------------------------

func (h *Handlers) apiListSamples(w http.ResponseWriter, r *http.Request) {
	limit, offset := ParsePagination(r.URL.Query().Get("limit"), r.URL.Query().Get("offset"))
	p := APIListParams{
		Q:      r.URL.Query().Get("q"),
		Owner:  r.URL.Query().Get("owner"),
		Status: r.URL.Query().Get("status"),
		From:   r.URL.Query().Get("from"),
		To:     r.URL.Query().Get("to"),
		Limit:  limit,
		Offset: offset,
	}

	result, err := h.svc.APIListSamples(r.Context(), p)
	if err != nil {
		log.Printf("samples api: listSamples error: %v", err)
		if errors.Is(err, web.ErrValidation) {
			writeError(w, http.StatusBadRequest, strings.TrimSuffix(err.Error(), ": "+web.ErrValidation.Error()))
			return
		}
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  result.Items,
		"total":  result.Total,
		"limit":  result.Limit,
		"offset": result.Offset,
	})
}

func (h *Handlers) apiGetSample(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	detail, err := h.svc.APIGetSample(r.Context(), id)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sample not found")
			return
		}
		log.Printf("samples api: getSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

type sampleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Keywords    string `json:"keywords"`
	Owner       string `json:"owner"`
	SamplePrep  string `json:"sample_prep"`
	Status      string `json:"status,omitempty"`
	OwnerUserID *int   `json:"owner_user_id,omitempty"`
}

func (h *Handlers) apiCreateSample(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req sampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name must be 100 characters or fewer")
		return
	}

	// Default owner is the session user. Admins may supply a different owner_user_id
	// or omit it entirely to create an ownerless (admin-only) sample.
	ownerUserIDPtr := &session.UserID
	ownerName := session.Username // non-admins always get their own username as owner
	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("samples api: createSample admin check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if isAdmin {
		ownerUserIDPtr = req.OwnerUserID // nil = ownerless; explicit ID overrides
		ownerName = req.Owner
		if ownerUserIDPtr != nil {
			// Always resolve from DB so name and ID stay consistent.
			ownerName, err = h.svc.GetUsernameByID(r.Context(), *ownerUserIDPtr)
			if err != nil {
				log.Printf("samples api: createSample resolve owner error: %v", err)
				writeError(w, http.StatusInternalServerError, "database error")
				return
			}
			if ownerName == "" {
				writeError(w, http.StatusBadRequest, "owner_user_id does not refer to an existing user")
				return
			}
		}
	}

	// Only admins may set an initial status; everyone else gets the DB default.
	status := ""
	if isAdmin {
		status = normalizeStatusInput(req.Status)
	}

	item, err := h.svc.APICreateSample(r.Context(),
		req.Name, req.Description, req.Keywords, ownerName, req.SamplePrep, status, ownerUserIDPtr)
	if err != nil {
		log.Printf("samples api: createSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("created"), activity.TargetType("sample"), item.ID, item.Name)
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handlers) apiUpdateSample(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := h.svc.CheckOwner(r.Context(), id, session.UserID); err != nil {
		writeOwnerError(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req sampleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name must be 100 characters or fewer")
		return
	}

	existing, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sample not found")
			return
		}
		log.Printf("samples api: updateSample lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	// Non-admins cannot change owner_user_id or the displayed owner name;
	// both are forced to the caller's own identity.
	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("samples api: updateSample admin check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	ownerUserID := req.OwnerUserID
	ownerName := req.Owner
	if !isAdmin {
		ownerUserID = &session.UserID
		ownerName = session.Username
	} else if ownerUserID != nil {
		// Resolve name from DB so name and ID stay consistent.
		resolved, resolveErr := h.svc.GetUsernameByID(r.Context(), *ownerUserID)
		if resolveErr != nil {
			log.Printf("samples api: updateSample resolve owner error: %v", resolveErr)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if resolved == "" {
			writeError(w, http.StatusBadRequest, "owner_user_id does not refer to an existing user")
			return
		}
		ownerName = resolved
	}

	// Only admins may change status; non-admin callers leave it untouched.
	status := ""
	if isAdmin {
		status = normalizeStatusInput(req.Status)
	}

	if err := h.svc.APIUpdateSample(r.Context(), id, req.Name, req.Description, req.Keywords, ownerName, req.SamplePrep, status, ownerUserID); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sample not found")
			return
		}
		log.Printf("samples api: updateSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	activityLabel := sampleUpdateActivityLabel(existing, req.Name, req.Description, req.Keywords, req.SamplePrep, ownerUserID, ownerName, status)
	h.logActivity(r.Context(), session, activity.Verb("updated"), activity.TargetType("sample"), id, activityLabel)

	// Re-fetch and return the updated detail.
	h.apiGetSample(w, r, sampleID)
}

func (h *Handlers) apiDeleteSample(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := h.svc.CheckOwner(r.Context(), id, session.UserID); err != nil {
		writeOwnerError(w, err)
		return
	}

	// Look up the sample name for the activity-log label before deleting.
	var sampleName string
	if s, getErr := h.svc.GetByID(r.Context(), id); getErr == nil {
		sampleName = s.Name
	}

	// APIDeleteSample collects file paths and removes both the sample row and
	// its attachment rows, returning the paths for disk cleanup.
	filePaths, err := h.svc.APIDeleteSample(r.Context(), id)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sample not found")
			return
		}
		log.Printf("samples api: deleteSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("deleted"), activity.TargetType("sample"), id, sampleName)

	// Remove attachment files from disk.
	for _, fp := range filePaths {
		if fp == "" {
			continue
		}
		if removeErr := removeFile(h.resolveAppPath(fp)); removeErr != nil {
			log.Printf("samples api: deleteSample remove file %q error: %v", fp, removeErr)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Fabrication steps API
// ---------------------------------------------------------------------------

func (h *Handlers) apiListSteps(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	exists, err := h.svc.SampleExistsByID(r.Context(), id)
	if err != nil {
		log.Printf("samples api: listSteps existence check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "sample not found")
		return
	}

	steps, err := h.svc.GetSteps(r.Context(), id)
	if err != nil {
		log.Printf("samples api: listSteps error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	items := make([]APIFabricationStep, 0, len(steps))
	for _, s := range steps {
		items = append(items, FabricationStepToAPI(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  len(items),
		"limit":  0,
		"offset": 0,
	})
}

type stepRequest struct {
	StepOrder     int     `json:"step_order"`
	StepDate      string  `json:"step_date"`
	PersonUserID  *int    `json:"person_user_id"`
	Description   string  `json:"description"`
	EquipmentID   *int    `json:"equipment_id"`
	EquipmentText *string `json:"equipment_text"`
}

func stepRequestToStep(req stepRequest) (FabricationStep, error) {
	step := FabricationStep{
		StepOrder:     req.StepOrder,
		PersonUserID:  req.PersonUserID,
		Description:   req.Description,
		EquipmentID:   req.EquipmentID,
		EquipmentText: req.EquipmentText,
	}
	if req.StepDate != "" {
		t, err := time.Parse("2006-01-02", req.StepDate)
		if err != nil {
			return FabricationStep{}, fmt.Errorf("invalid step_date: must be YYYY-MM-DD")
		}
		step.StepDate = &t
	}
	return step, nil
}

func (h *Handlers) apiAddStep(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := h.svc.CheckOwner(r.Context(), id, session.UserID); err != nil {
		writeOwnerError(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req stepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	step, err := stepRequestToStep(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.svc.AddStep(r.Context(), id, step)
	if err != nil {
		log.Printf("samples api: addStep error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("added_step"), activity.TargetType("fabrication_step"), created.ID, created.Description)
	writeJSON(w, http.StatusCreated, FabricationStepToAPI(created))
}

func (h *Handlers) apiUpdateStep(w http.ResponseWriter, r *http.Request, stepID string) {
	id, err := strconv.Atoi(stepID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}

	existing, err := h.svc.GetStepByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		log.Printf("samples api: updateStep lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := h.svc.CheckOwner(r.Context(), existing.SampleID, session.UserID); err != nil {
		writeOwnerError(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req stepRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	step, parseErr := stepRequestToStep(req)
	if parseErr != nil {
		writeError(w, http.StatusBadRequest, parseErr.Error())
		return
	}

	if err := h.svc.UpdateStep(r.Context(), id, step); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		log.Printf("samples api: updateStep error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("edited_step"), activity.TargetType("fabrication_step"), id, step.Description)

	updated, err := h.svc.GetStepByID(r.Context(), id)
	if err != nil {
		log.Printf("samples api: updateStep fetch error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusOK, FabricationStepToAPI(updated))
}

func (h *Handlers) apiDeleteStep(w http.ResponseWriter, r *http.Request, stepID string) {
	id, err := strconv.Atoi(stepID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid step id")
		return
	}

	existing, err := h.svc.GetStepByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		log.Printf("samples api: deleteStep lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := h.svc.CheckOwner(r.Context(), existing.SampleID, session.UserID); err != nil {
		writeOwnerError(w, err)
		return
	}

	if err := h.svc.DeleteStep(r.Context(), id); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "step not found")
			return
		}
		log.Printf("samples api: deleteStep error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("deleted"), activity.TargetType("fabrication_step"), id, existing.Description)
	w.WriteHeader(http.StatusNoContent)
}

// removeFile removes a file, ignoring ErrNotExist.
func removeFile(path string) error {
	if path == "" {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Attachments API
// ---------------------------------------------------------------------------

// apiListAttachments handles GET /api/v1/samples/{id}/attachments.
func (h *Handlers) apiListAttachments(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	exists, err := h.svc.SampleExistsByID(r.Context(), id)
	if err != nil {
		log.Printf("samples api: listAttachments existence check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, "sample not found")
		return
	}

	atts, err := h.svc.GetAttachments(r.Context(), id)
	if err != nil {
		log.Printf("samples api: listAttachments error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	items := make([]APIAttachment, 0, len(atts))
	for _, a := range atts {
		items = append(items, AttachmentToAPI(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  items,
		"total":  len(items),
		"limit":  0,
		"offset": 0,
	})
}

// apiUploadAttachments handles POST /api/v1/samples/{id}/attachments.
// Accepts multipart/form-data with a "files" field (one or more). Returns
// the newly-created attachment records plus structured error details for
// any rejected files.
func (h *Handlers) apiUploadAttachments(w http.ResponseWriter, r *http.Request, sampleID string) {
	id, err := strconv.Atoi(sampleID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sample id")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if err := h.svc.CheckOwner(r.Context(), id, session.UserID); err != nil {
		writeOwnerError(w, err)
		return
	}

	// 100 MiB hard cap on the whole request; 32 MiB kept in memory.
	const maxUploadBytes = 100 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Printf("samples api: parse multipart form: %v", err)
		writeError(w, http.StatusBadRequest, "unable to parse request (max 100 MiB)")
		return
	}
	defer r.MultipartForm.RemoveAll()

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		writeError(w, http.StatusBadRequest, `no files provided (expected multipart field "files")`)
		return
	}

	type uploadError struct {
		Filename string `json:"filename"`
		Error    string `json:"error"`
	}

	var (
		created     []APIAttachment
		errs        []uploadError
		insertedIDs = make(map[int]struct{})
	)
	for _, fh := range fileHeaders {
		f, openErr := fh.Open()
		if openErr != nil {
			errs = append(errs, uploadError{Filename: fh.Filename, Error: "could not open file"})
			continue
		}
		fp, saveErr := h.saveUploadedFile(f, fh.Filename)
		f.Close()
		if saveErr != nil {
			errs = append(errs, uploadError{Filename: fh.Filename, Error: "could not save file"})
			continue
		}
		attID, dbErr := h.svc.AddAttachment(r.Context(), id, fp)
		if dbErr != nil {
			_ = removeFile(h.resolveAppPath(fp))
			errs = append(errs, uploadError{Filename: fh.Filename, Error: "could not store attachment record"})
			continue
		}
		h.logActivity(r.Context(), session, activity.Verb("uploaded"), activity.TargetType("attachment"), attID, fh.Filename)
		insertedIDs[attID] = struct{}{}
	}

	// Re-fetch to get full metadata (MIME type, original name) for inserted rows.
	// Filter by the IDs we just inserted to avoid returning unrelated attachments
	// that may have been added concurrently.
	atts, listErr := h.svc.GetAttachments(r.Context(), id)
	if listErr != nil {
		log.Printf("samples api: uploadAttachments refetch error: %v", listErr)
		writeError(w, http.StatusInternalServerError, "attachments were saved but could not be listed")
		return
	}
	for _, att := range atts {
		if _, ok := insertedIDs[att.ID]; ok {
			created = append(created, AttachmentToAPI(att))
		}
	}

	status := http.StatusCreated
	if len(created) == 0 {
		status = http.StatusBadRequest
	} else if len(errs) > 0 {
		status = http.StatusMultiStatus // 207: partial success
	}

	writeJSON(w, status, map[string]any{
		"created": created,
		"errors":  errs,
	})
}

// apiDownloadAttachment handles GET /api/v1/attachments/{id}.
// Returns the raw file bytes with the correct Content-Type.
func (h *Handlers) apiDownloadAttachment(w http.ResponseWriter, r *http.Request, attachmentID int) {
	filePath, err := h.svc.AttachmentFilePath(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		log.Printf("samples api: downloadAttachment filepath error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	resolved := h.resolveAppPath(filePath)
	if resolved == "" {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	// Reuse the same Content-Disposition logic as the non-API download path:
	// images render inline, everything else is forced as a download.
	if isImage(mime.TypeByExtension(filepath.Ext(filePath))) {
		setInlineHeaders(w, originalFilenameFromPath(filePath))
	} else {
		setDownloadHeaders(w, originalFilenameFromPath(filePath))
	}
	http.ServeFile(w, r, resolved)
}

// apiDeleteAttachment handles DELETE /api/v1/attachments/{id}.
func (h *Handlers) apiDeleteAttachment(w http.ResponseWriter, r *http.Request, attachmentID int) {
	sampleID, _, err := h.svc.AttachmentInfo(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		log.Printf("samples api: deleteAttachment info error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if ownerErr := h.svc.CheckOwner(r.Context(), sampleID, session.UserID); ownerErr != nil {
		writeOwnerError(w, ownerErr)
		return
	}

	filePath, err := h.svc.DeleteAttachment(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		log.Printf("samples api: deleteAttachment error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("deleted"), activity.TargetType("attachment"), attachmentID, originalFilenameFromPath(filePath))
	if filePath != "" {
		if rmErr := removeFile(h.resolveAppPath(filePath)); rmErr != nil {
			log.Printf("samples api: deleteAttachment remove file %q: %v", filePath, rmErr)
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
