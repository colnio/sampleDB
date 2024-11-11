package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type UserAccess struct {
	UserID          int         `json:"user_id"`
	Username        string      `json:"username"`
	Approved        bool        `json:"approved"`
	Admin           bool        `json:"admin"`
	CreatedAt       string      `json:"created_at"`
	EquipmentAccess []Equipment `json:"equipment_access"`
}

type AdminPageData struct {
	BasePageData
	Users     []UserAccess
	Equipment []Equipment
	Error     string
	Success   string
}

func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)

	// Get all equipment first
	equipment, err := getAllEquipment()
	if err != nil {
		http.Error(w, "Error getting equipment", http.StatusInternalServerError)
		return
	}

	// Get all users and their permissions
	rows, err := dbPool.Query(context.Background(), `
        SELECT DISTINCT 
            u.user_id, 
            u.username, 
            u.is_approved, 
            u.admin,
            array_remove(array_agg(uep.equipment_id), NULL) as equipment_ids
        FROM users u
        LEFT JOIN user_equipment_permissions uep ON u.user_id = uep.user_id
        GROUP BY u.user_id, u.username
        ORDER BY u.username`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []UserAccess
	for rows.Next() {
		var u UserAccess
		var equipmentIDs []int
		err := rows.Scan(&u.UserID, &u.Username, &u.Approved,
			&u.Admin, &equipmentIDs)
		if err != nil {
			continue
		}

		// Map equipment permissions
		u.EquipmentAccess = make([]Equipment, 0)
		for _, eqID := range equipmentIDs {
			for _, eq := range equipment {
				if eq.ID == eqID {
					u.EquipmentAccess = append(u.EquipmentAccess, eq)
					break
				}
			}
		}
		users = append(users, u)
	}

	data := AdminPageData{
		BasePageData: BasePageData{
			Username: session.Username,
			UserID:   session.UserID,
			IsAdmin:  true,
		},
		Users:     users,
		Equipment: equipment,
		Error:     r.URL.Query().Get("error"),
		Success:   r.URL.Query().Get("success"),
	}

	tmpl, err := parseTemplates("templates/admin.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleUpdateAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		UserID    int   `json:"user_id"`
		Approved  bool  `json:"approved"`
		Equipment []int `json:"equipment"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Start a transaction
	tx, err := dbPool.Begin(context.Background())
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	// Update approved status
	_, err = tx.Exec(context.Background(),
		"UPDATE users SET is_approved = $1 WHERE user_id = $2",
		data.Approved, data.UserID)
	if err != nil {
		http.Error(w, "Error updating approval status", http.StatusInternalServerError)
		return
	}

	// Remove all existing equipment permissions
	_, err = tx.Exec(context.Background(),
		"DELETE FROM user_equipment_permissions WHERE user_id = $1",
		data.UserID)
	if err != nil {
		http.Error(w, "Error updating equipment permissions", http.StatusInternalServerError)
		return
	}

	// Add new equipment permissions
	for _, equipID := range data.Equipment {
		_, err = tx.Exec(context.Background(),
			"INSERT INTO user_equipment_permissions (user_id, equipment_id) VALUES ($1, $2)",
			data.UserID, equipID)
		if err != nil {
			continue
		}
	}

	// Commit transaction
	if err = tx.Commit(context.Background()); err != nil {
		http.Error(w, "Error committing changes", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := r.Context().Value("user").(Session)

		var isAdmin bool
		err := dbPool.QueryRow(context.Background(),
			"SELECT admin FROM users WHERE user_id = $1",
			session.UserID).Scan(&isAdmin)

		if err != nil || !isAdmin {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

// Handler for setting admin status
func handleSetAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.FormValue("user_id")
	isAdmin := r.FormValue("is_admin") == "true"

	_, err := dbPool.Exec(context.Background(),
		"UPDATE users SET admin = $1 WHERE user_id = $2",
		isAdmin, userID)

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?success=Admin+status+updated", http.StatusSeeOther)
}
