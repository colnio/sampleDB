package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/auth"
)

// renderFabricationStepsSection re-renders the fabrication steps panel as an HTMX partial.
// editingStep is the step_id to show in inline edit mode (0 = none).
func renderFabricationStepsSection(w http.ResponseWriter, r *http.Request, session auth.Session, sampleIDStr, flash, errMsg string, editingStep int) {
	data, err := loadSampleDetailData(r.Context(), session, sampleIDStr)
	if err != nil {
		http.Error(w, "Sample not found", http.StatusNotFound)
		return
	}
	data.Flash = flash
	data.Error = errMsg
	data.IsPartial = true
	data.EditingStep = editingStep

	tmpl, err := parseTemplates("templates/fabrication_steps.html", "templates/step_form.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "fabrication_steps_panel", data); err != nil {
		http.Error(w, "Error rendering steps panel", http.StatusInternalServerError)
	}
}

// parseStepForm reads step fields from a parsed form. r.ParseForm must have been called first.
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

	step.Description = r.FormValue("description")
	return step, nil
}

// handleFabricationStepsEndpoint handles GET (render panel) and POST (add step) for /samples/{id}/steps.
func handleFabricationStepsEndpoint(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	sampleIDStr := parts[0]

	switch r.Method {
	case http.MethodGet:
		renderFabricationStepsSection(w, r, session, sampleIDStr, "", "", 0)

	case http.MethodPost:
		sampleIDInt, err := strconv.Atoi(sampleIDStr)
		if err != nil {
			http.Error(w, "Invalid sample ID", http.StatusBadRequest)
			return
		}

		if err := requireSampleOwner(r, sampleIDInt, session.UserID); err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if err := r.ParseForm(); err != nil {
			renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Invalid form submission.", 0)
			return
		}

		step, parseErr := parseStepForm(r)
		if parseErr != nil {
			renderFabricationStepsSection(w, r, session, sampleIDStr, "", parseErr.Error(), 0)
			return
		}

		if _, err := addFabricationStep(sampleIDInt, step); err != nil {
			renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Failed to add step.", 0)
			return
		}

		if isHTMXRequest(r) {
			renderFabricationStepsSection(w, r, session, sampleIDStr, "Step added", "", 0)
			return
		}
		http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStepAction dispatches requests under /steps/{id}/edit and /steps/{id}/delete.
func handleStepAction(w http.ResponseWriter, r *http.Request) {
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
		handleFabricationStepEdit(w, r, stepIDStr)
	case "delete":
		handleFabricationStepDelete(w, r, stepIDStr)
	default:
		http.Error(w, "Unknown step action", http.StatusNotFound)
	}
}

// handleFabricationStepEdit handles GET (inline edit mode) and POST (save update) for /steps/{id}/edit.
func handleFabricationStepEdit(w http.ResponseWriter, r *http.Request, stepIDStr string) {
	session := auth.MustSessionFromContext(r.Context())
	stepID, err := strconv.Atoi(stepIDStr)
	if err != nil {
		http.Error(w, "Invalid step ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		existing, err := getStepByID(stepID)
		if err != nil {
			http.Error(w, "Step not found", http.StatusNotFound)
			return
		}
		if err := requireSampleOwner(r, existing.SampleID, session.UserID); err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		sampleIDStr := strconv.Itoa(existing.SampleID)
		renderFabricationStepsSection(w, r, session, sampleIDStr, "", "", stepID)

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", http.StatusBadRequest)
			return
		}

		// Re-fetch the step to get sample_id for ownership check.
		existing, err := getStepByID(stepID)
		if err != nil {
			http.Error(w, "Step not found", http.StatusNotFound)
			return
		}
		if err := requireSampleOwner(r, existing.SampleID, session.UserID); err != nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		step, parseErr := parseStepForm(r)
		if parseErr != nil {
			http.Error(w, parseErr.Error(), http.StatusBadRequest)
			return
		}

		if err := updateFabricationStep(stepID, step); err != nil {
			http.Error(w, "Failed to update step", http.StatusInternalServerError)
			return
		}

		sampleIDStr := strconv.Itoa(existing.SampleID)

		if isHTMXRequest(r) {
			renderFabricationStepsSection(w, r, session, sampleIDStr, "Step updated", "", 0)
			return
		}
		http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleFabricationStepDelete handles POST /steps/{id}/delete.
func handleFabricationStepDelete(w http.ResponseWriter, r *http.Request, stepIDStr string) {
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

	// Fetch sample_id before deleting so we can re-render the panel
	existing, err := getStepByID(stepID)
	if err != nil {
		http.Error(w, "Step not found", http.StatusNotFound)
		return
	}
	if err := requireSampleOwner(r, existing.SampleID, session.UserID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	sampleIDStr := strconv.Itoa(existing.SampleID)

	if err := deleteFabricationStep(stepID); err != nil {
		renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Failed to delete step.", 0)
		return
	}

	if isHTMXRequest(r) {
		renderFabricationStepsSection(w, r, session, sampleIDStr, "Step deleted", "", 0)
		return
	}
	http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)
}

// handleFabricationStepsReorder handles POST /samples/{id}/steps/reorder.
// Expects repeated "step_id" form values in the desired new order.
func handleFabricationStepsReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/samples/"), "/")
	sampleIDStr := parts[0]
	sampleIDInt, err := strconv.Atoi(sampleIDStr)
	if err != nil {
		http.Error(w, "Invalid sample ID", http.StatusBadRequest)
		return
	}

	if err := requireSampleOwner(r, sampleIDInt, session.UserID); err != nil {
		log.Printf("handleFabricationStepsReorder: ownership check failed for sample %d user %d: %v", sampleIDInt, session.UserID, err)
		http.Error(w, "Forbidden", http.StatusForbidden)
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

	if len(orderedIDs) > 0 {
		if err := reorderFabricationSteps(sampleIDInt, orderedIDs); err != nil {
			renderFabricationStepsSection(w, r, session, sampleIDStr, "", "Failed to reorder steps.", 0)
			return
		}
	}

	if isHTMXRequest(r) {
		renderFabricationStepsSection(w, r, session, sampleIDStr, "", "", 0)
		return
	}
	http.Redirect(w, r, "/samples/"+sampleIDStr, http.StatusSeeOther)
}
