package main

import (
	"context"
	"net/http"
	"strings"

	"sampleDB/internal/auth"
)

func samplePrepHandler(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	trimmed := strings.TrimPrefix(r.URL.Path, "/samples/prep/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		http.Error(w, "Sample ID required", http.StatusBadRequest)
		return
	}

	parts := strings.Split(trimmed, "/")
	sampleID := parts[0]
	var action string
	if len(parts) > 1 {
		action = parts[1]
	}

	switch r.Method {
	case http.MethodGet:
		renderSamplePrepSection(w, r, session, sampleID, "", "", action == "edit")
		return

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			renderSamplePrepSection(w, r, session, sampleID, "", "Invalid form submission.", true)
			return
		}

		prep := r.FormValue("sample_prep")
		if _, err := dbPool.Exec(context.Background(),
			"UPDATE samples SET sample_prep = $1 WHERE sample_id = $2",
			prep, sampleID); err != nil {
			renderSamplePrepSection(w, r, session, sampleID, "", "Failed to update sample preparation.", true)
			return
		}

		if isHTMXRequest(r) {
			renderSamplePrepSection(w, r, session, sampleID, "Preparation updated", "", false)
			return
		}

		http.Redirect(w, r, "/samples/"+sampleID, http.StatusSeeOther)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
