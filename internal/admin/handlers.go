package admin

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// Handlers groups all HTTP handler methods for the admin domain.
type Handlers struct {
	svc      Service
	renderer *web.Renderer
	authMgr  *auth.Manager
}

// NewHandlers constructs a Handlers value.
func NewHandlers(svc Service, r *web.Renderer, a *auth.Manager) *Handlers {
	return &Handlers{svc: svc, renderer: r, authMgr: a}
}

// AdminPage handles GET /admin.
func (h *Handlers) AdminPage(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	equipment, err := h.svc.GetAllEquipment(r.Context())
	if err != nil {
		http.Error(w, "Error getting equipment", http.StatusInternalServerError)
		return
	}

	groups, err := h.svc.GetAllGroups(r.Context())
	if err != nil {
		http.Error(w, "Error getting groups", http.StatusInternalServerError)
		return
	}

	users, err := h.svc.GetAllUsers(r.Context(), equipment)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	notifs, err := h.svc.ListNotifications(r.Context())
	if err != nil {
		log.Printf("admin: list notifications: %v", err)
		notifs = nil
	}

	data := AdminPageData{
		BasePageData: BasePageData{
			Username:  session.Username,
			UserID:    session.UserID,
			IsAdmin:   true, // guaranteed by RequireAdmin middleware
			CSRFToken: session.CSRFToken,
		},
		Users:         users,
		Equipment:     equipment,
		Groups:        groups,
		Notifications: notifs,
		Error:         r.URL.Query().Get("error"),
		Success:       r.URL.Query().Get("success"),
	}

	if err := h.renderer.Render(w, "admin", data); err != nil {
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// UpdateAccess handles POST /admin/update-access.
func (h *Handlers) UpdateAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		UserID    int     `json:"user_id"`
		Approved  bool    `json:"approved"`
		GroupName *string `json:"group_name"`
		Equipment []int   `json:"equipment"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("admin: error parsing access update JSON: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateAccess(r.Context(), UpdateAccessParams{
		UserID:    payload.UserID,
		Approved:  payload.Approved,
		GroupName: payload.GroupName,
		Equipment: payload.Equipment,
	}); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		log.Printf("admin: error updating access: %v", err)
		http.Error(w, "Error updating access", http.StatusInternalServerError)
		return
	}

	if h.authMgr != nil && !payload.Approved {
		if err := h.authMgr.RevokeUserSessions(r.Context(), payload.UserID); err != nil {
			log.Printf("admin: UpdateAccess revoke sessions: %v", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// SetAdmin handles POST /admin/set-admin.
func (h *Handlers) SetAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	session := auth.MustSessionFromContext(r.Context())

	userID, err := strconv.Atoi(r.FormValue("user_id"))
	if err != nil {
		http.Error(w, "Invalid user id", http.StatusBadRequest)
		return
	}

	if userID == session.UserID {
		http.Error(w, "Cannot change your own admin status", http.StatusBadRequest)
		return
	}

	isAdmin := r.FormValue("is_admin") == "true"

	if err := h.svc.SetAdmin(r.Context(), userID, isAdmin); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
		} else {
			http.Error(w, "Database error", http.StatusInternalServerError)
		}
		return
	}

	if h.authMgr != nil && !isAdmin {
		if err := h.authMgr.RevokeUserSessions(r.Context(), userID); err != nil {
			log.Printf("admin: SetAdmin revoke sessions: %v", err)
		}
	}

	http.Redirect(w, r, "/admin?success=Admin+status+updated", http.StatusSeeOther)
}

// DeleteUser handles POST /admin/delete-user.
// Accepts both form-encoded and JSON request bodies.
func (h *Handlers) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	session := auth.MustSessionFromContext(r.Context())

	var userID int
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") {
		var payload struct {
			UserID int `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		userID = payload.UserID
	} else {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form data", http.StatusBadRequest)
			return
		}
		parsed, err := strconv.Atoi(r.FormValue("user_id"))
		if err != nil {
			http.Error(w, "Invalid user id", http.StatusBadRequest)
			return
		}
		userID = parsed
	}

	if userID == 0 {
		http.Error(w, "User id required", http.StatusBadRequest)
		return
	}
	if userID == session.UserID {
		http.Error(w, "You cannot remove your own account", http.StatusBadRequest)
		return
	}

	rowsAffected, err := h.svc.DeleteUser(r.Context(), userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if h.authMgr != nil {
		if err := h.authMgr.RevokeUserSessions(r.Context(), userID); err != nil {
			log.Printf("admin: DeleteUser revoke sessions: %v", err)
		}
	}

	if strings.Contains(contentType, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
		return
	}
	http.Redirect(w, r, "/admin?success=User+removed", http.StatusSeeOther)
}

// ResetPassword handles POST /admin/reset-password.
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload struct {
		UserID int `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	if payload.UserID <= 0 {
		http.Error(w, "User id required", http.StatusBadRequest)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	if payload.UserID == session.UserID {
		http.Error(w, "Cannot reset your own password via admin panel", http.StatusBadRequest)
		return
	}

	password, err := h.svc.ResetPassword(r.Context(), payload.UserID)
	if err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to reset password", http.StatusInternalServerError)
		return
	}

	if h.authMgr != nil {
		if err := h.authMgr.RevokeUserSessions(r.Context(), payload.UserID); err != nil {
			log.Printf("admin: ResetPassword revoke sessions for user %d: %v", payload.UserID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"password": password,
		"note":     "User must change password on next login",
	})
}

// AddEquipment handles POST /admin/add-equipment.
func (h *Handlers) AddEquipment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/admin?error=Equipment+name+is+required", http.StatusSeeOther)
		return
	}

	if err := h.svc.AddEquipment(r.Context(), name); err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+add+equipment", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Equipment+added+successfully", http.StatusSeeOther)
}

// DeleteEquipment handles POST /admin/delete-equipment/{id}.
func (h *Handlers) DeleteEquipment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr, err := web.ExtractIDFromPath(r.URL.Path, "/admin/delete-equipment/")
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	equipmentID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid equipment ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteEquipment(r.Context(), equipmentID); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Error(w, "Equipment not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error deleting equipment", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
}

// SetEquipmentStatus handles POST /admin/equipment/{id}/status.
// It accepts a form-encoded "status" field and redirects back to /admin.
func (h *Handlers) SetEquipmentStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	equipmentID, err := extractIDFromPrefixAndSuffix(r.URL.Path, "/admin/equipment/", "/status")
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	status := strings.TrimSpace(r.FormValue("status"))
	if status == "" {
		http.Redirect(w, r, "/admin?error=Invalid+status+value", http.StatusSeeOther)
		return
	}
	if err := h.svc.SetEquipmentStatus(r.Context(), equipmentID, status); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Redirect(w, r, "/admin?error=Equipment+not+found", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin?error=Failed+to+update+status", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Equipment+status+updated", http.StatusSeeOther)
}

// AddEquipmentGroup handles POST /admin/equipment/{id}/groups/add.
// Accepts a form-encoded "group_id" field.
func (h *Handlers) AddEquipmentGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	equipmentID, err := extractIDFromPrefixAndSuffix(r.URL.Path, "/admin/equipment/", "/groups/add")
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("group_id")))
	if err != nil || groupID <= 0 {
		http.Redirect(w, r, "/admin?error=Select+a+group", http.StatusSeeOther)
		return
	}

	if err := h.svc.AddEquipmentGroup(r.Context(), equipmentID, groupID); err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+grant+group+access", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Group+granted", http.StatusSeeOther)
}

// RemoveEquipmentGroup handles POST /admin/equipment/{id}/groups/remove.
// Accepts a form-encoded "group_id" field.
func (h *Handlers) RemoveEquipmentGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	equipmentID, err := extractIDFromPrefixAndSuffix(r.URL.Path, "/admin/equipment/", "/groups/remove")
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}
	groupID, err := strconv.Atoi(strings.TrimSpace(r.FormValue("group_id")))
	if err != nil || groupID <= 0 {
		http.Redirect(w, r, "/admin?error=Select+a+group", http.StatusSeeOther)
		return
	}

	if err := h.svc.RemoveEquipmentGroup(r.Context(), equipmentID, groupID); err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+revoke+group+access", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Group+revoked", http.StatusSeeOther)
}

// extractIDFromPrefixAndSuffix parses an integer ID from a URL path of the
// form "<prefix><ID><suffix>". It returns an error when the path does not
// match the expected shape or the ID is not numeric.
func extractIDFromPrefixAndSuffix(path, prefix, suffix string) (int, error) {
	if !strings.HasPrefix(path, prefix) {
		return 0, fmt.Errorf("path %q missing prefix %q", path, prefix)
	}
	remainder := strings.TrimPrefix(path, prefix)
	if !strings.HasSuffix(remainder, suffix) {
		return 0, fmt.Errorf("path %q missing suffix %q", path, suffix)
	}
	idStr := strings.TrimSuffix(remainder, suffix)
	if idStr == "" {
		return 0, fmt.Errorf("empty id in path %q", path)
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q: %w", idStr, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("non-positive id %d in path %q", id, path)
	}
	return id, nil
}

// AddGroup handles POST /admin/add-group.
func (h *Handlers) AddGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/admin?error=Group+name+is+required", http.StatusSeeOther)
		return
	}

	if err := h.svc.AddGroup(r.Context(), name); err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+add+group", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Group+added", http.StatusSeeOther)
}

// DeleteGroup handles POST /admin/delete-group/{id}.
func (h *Handlers) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	groupID, err := extractIDFromPrefixAndSuffix(r.URL.Path, "/admin/delete-group/", "")
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteGroup(r.Context(), groupID); err != nil {
		if errors.Is(err, web.ErrNotFound) {
			http.Redirect(w, r, "/admin?error=Unknown+group", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/admin?error=Failed+to+delete+group", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Group+removed", http.StatusSeeOther)
}

// EquipmentReport handles GET /admin/equipment-report and returns a CSV download.
func (h *Handlers) EquipmentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	equipmentIDStr := r.URL.Query().Get("id")
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	equipmentID, err := strconv.Atoi(equipmentIDStr)
	if equipmentIDStr == "" || err != nil {
		http.Error(w, "Invalid or missing equipment ID", http.StatusBadRequest)
		return
	}
	if startDate == "" || endDate == "" {
		http.Error(w, "Missing start or end date", http.StatusBadRequest)
		return
	}

	const dateLayout = "2006-01-02"
	if _, err := time.Parse(dateLayout, startDate); err != nil {
		http.Error(w, "Invalid start date, use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	if _, err := time.Parse(dateLayout, endDate); err != nil {
		http.Error(w, "Invalid end date, use YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	if startDate > endDate {
		http.Error(w, "Start date must be before end date", http.StatusBadRequest)
		return
	}

	records, err := h.svc.GetEquipmentReport(r.Context(), equipmentID, startDate, endDate)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=equipment_usage.csv")

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	headers := []string{"Start Time", "End Time", "Duration (hours)", "User", "Group", "Equipment", "Purpose"}
	if err := csvWriter.Write(headers); err != nil {
		return
	}

	for _, rec := range records {
		duration := rec.EndTime.Sub(rec.StartTime).Hours()
		row := []string{
			rec.StartTime.Format("2006-01-02 15:04"),
			rec.EndTime.Format("2006-01-02 15:04"),
			fmt.Sprintf("%.2f", duration),
			rec.Username,
			rec.GroupName,
			rec.EquipmentName,
			rec.Purpose,
		}
		if err := csvWriter.Write(row); err != nil {
			log.Printf("csv write error: %v", err)
			return
		}
	}
}

// APIUsers handles GET /api/v1/users.
func (h *Handlers) APIUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		web.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	users, err := h.svc.ListAPIUsers(r.Context())
	if err != nil {
		web.JSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	web.JSON(w, http.StatusOK, map[string]any{
		"items": users,
		"total": len(users),
	})
}

// APIEquipment handles GET /api/v1/equipment.
func (h *Handlers) APIEquipment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		web.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	equipment, err := h.svc.GetAllEquipment(r.Context())
	if err != nil {
		web.JSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}

	items := make([]APIEquipment, 0, len(equipment))
	for _, item := range equipment {
		items = append(items, APIEquipment{
			ID:          item.ID,
			Name:        item.Name,
			Description: item.Description,
			Location:    item.Location,
		})
	}

	web.JSON(w, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// ListNotifications handles GET /api/notifications — returns active notifications as JSON.
func (h *Handlers) ListNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		web.JSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	notifs, err := h.svc.ListNotifications(r.Context())
	if err != nil {
		log.Printf("admin: list notifications: %v", err)
		web.JSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	web.JSON(w, http.StatusOK, notifs)
}

// CreateNotification handles POST /admin/notifications.
func (h *Handlers) CreateNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := auth.MustSessionFromContext(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin?error=invalid+request", http.StatusSeeOther)
		return
	}
	msg := strings.TrimSpace(r.FormValue("message"))
	level := r.FormValue("level")
	if msg == "" {
		http.Redirect(w, r, "/admin?error=message+required", http.StatusSeeOther)
		return
	}
	if len(msg) > 2000 {
		http.Redirect(w, r, "/admin?error=message+too+long+(max+2000+characters)", http.StatusSeeOther)
		return
	}
	if level != "info" && level != "warning" && level != "critical" {
		level = "info"
	}
	if _, err := h.svc.CreateNotification(r.Context(), msg, level, session.Username); err != nil {
		log.Printf("admin: create notification: %v", err)
		http.Redirect(w, r, "/admin?error=could+not+save+notification", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Notification+posted", http.StatusSeeOther)
}

// DeleteNotification handles POST /admin/notifications/{id}/delete.
func (h *Handlers) DeleteNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	notifID, err := extractIDFromPrefixAndSuffix(r.URL.Path, "/admin/notifications/", "/delete")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	id := int64(notifID)
	if err := h.svc.DeleteNotification(r.Context(), id); err != nil {
		log.Printf("admin: delete notification %d: %v", id, err)
		http.Redirect(w, r, "/admin?error=could+not+delete+notification", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin?success=Notification+removed", http.StatusSeeOther)
}

