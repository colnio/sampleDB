package samples

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("samples api: writeJSON encode error: %v", err)
	}
}

// writeError writes a JSON error body with the given status code.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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
	default:
		writeError(w, http.StatusNotFound, "not found")
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
		From:   r.URL.Query().Get("from"),
		To:     r.URL.Query().Get("to"),
		Limit:  limit,
		Offset: offset,
	}

	result, err := h.svc.APIListSamples(r.Context(), p)
	if err != nil {
		log.Printf("samples api: listSamples error: %v", err)
		if strings.Contains(err.Error(), "invalid") {
			writeError(w, http.StatusBadRequest, err.Error())
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

	// Default owner is the session user. Admins may supply a different owner_user_id
	// or omit it entirely to create an ownerless (admin-only) sample.
	ownerUserIDPtr := &session.UserID
	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("samples api: createSample admin check error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if isAdmin {
		ownerUserIDPtr = req.OwnerUserID // nil = ownerless; explicit ID overrides
	}

	item, err := h.svc.APICreateSample(r.Context(),
		req.Name, req.Description, req.Keywords, req.Owner, ownerUserIDPtr)
	if err != nil {
		log.Printf("samples api: createSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
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

	if err := h.svc.APIUpdateSample(r.Context(), id, req.Name, req.Description, req.Keywords, req.Owner); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			writeError(w, http.StatusNotFound, "sample not found")
			return
		}
		log.Printf("samples api: updateSample error: %v", err)
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}

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

	// APIDeleteSample collects file paths before the DELETE (which CASCADE-removes attachment rows),
	// so disk cleanup can proceed even though DB rows are gone.
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
