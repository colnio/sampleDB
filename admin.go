package main

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/auth"
)

type UserAccess struct {
	UserID          int         `json:"user_id"`
	Username        string      `json:"username"`
	Approved        bool        `json:"approved"`
	Admin           bool        `json:"admin"`
	GroupName       string      `json:"group_name"`
	CreatedAt       string      `json:"created_at"`
	EquipmentAccess []Equipment `json:"equipment_access"`
}

type Group struct {
	ID   int
	Name string
}

type AdminPageData struct {
	BasePageData
	Users     []UserAccess
	Equipment []Equipment
	Groups    []Group
	Error     string
	Success   string
}

func getBasePageData(session auth.Session) (BasePageData, error) {
	var isAdmin bool
	err := dbPool.QueryRow(context.Background(),
		"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		session.UserID).Scan(&isAdmin)
	if err != nil {
		return BasePageData{}, err
	}

	return BasePageData{
		Username: session.Username,
		UserID:   session.UserID,
		IsAdmin:  isAdmin,
	}, nil
}
func handleAdminPage(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	baseData, err := getBasePageData(session)
	if err != nil {
		http.Error(w, "Error getting user data", http.StatusInternalServerError)
		return
	}

	equipment, err := getAllEquipment()
	if err != nil {
		http.Error(w, "Error getting equipment", http.StatusInternalServerError)
		return
	}

	groups, err := getAllGroups()
	if err != nil {
		http.Error(w, "Error getting groups", http.StatusInternalServerError)
		return
	}

	rows, err := dbPool.Query(context.Background(), `
        SELECT 
            u.user_id, 
            u.username, 
            u.is_approved, 
            u.admin,
            btrim(u."group") AS group_name,
            COALESCE(
                array_agg(uep.equipment_id) FILTER (WHERE uep.equipment_id IS NOT NULL),
                '{}'::int[]
            ) as equipment_ids
        FROM users u
        LEFT JOIN user_equipment_permissions uep ON u.user_id = uep.user_id
        WHERE COALESCE(u.deleted, false) = false
        GROUP BY u.user_id, u.username, u.is_approved, u.admin, group_name
        ORDER BY u.username`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []UserAccess
	for rows.Next() {
		var (
			u            UserAccess
			groupName    sql.NullString
			equipmentIDs []int
		)

		err := rows.Scan(&u.UserID, &u.Username, &u.Approved,
			&u.Admin, &groupName, &equipmentIDs)
		if err != nil {
			fmt.Println(err)
			continue
		}

		if groupName.Valid {
			u.GroupName = groupName.String
		}

		u.EquipmentAccess = make([]Equipment, 0)
		if equipmentIDs != nil {
			for _, eq := range equipment {
				for _, id := range equipmentIDs {
					if eq.ID == id {
						u.EquipmentAccess = append(u.EquipmentAccess, eq)
						break
					}
				}
			}
		}
		users = append(users, u)
	}

	data := AdminPageData{
		BasePageData: baseData,
		Users:        users,
		Equipment:    equipment,
		Groups:       groups,
		Error:        r.URL.Query().Get("error"),
		Success:      r.URL.Query().Get("success"),
	}

	tmpl, err := parseTemplates("templates/admin.html")
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleUpdateAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		UserID    int     `json:"user_id"`
		Approved  bool    `json:"approved"`
		GroupName *string `json:"group_name"`
		Equipment []int   `json:"equipment"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		fmt.Println("ON parsing json:", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	// fmt.Println(data)
	// Start a transaction
	tx, err := dbPool.Begin(context.Background())
	if err != nil {
		fmt.Println("ON starting transaction:", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	var groupParam interface{}
	if data.GroupName != nil {
		name := strings.TrimSpace(*data.GroupName)
		if name != "" {
			groupParam = name
		}
	}

	_, err = tx.Exec(context.Background(),
		`UPDATE users SET is_approved = $1, "group" = $2 WHERE user_id = $3 AND COALESCE(deleted, false) = false`,
		data.Approved, groupParam, data.UserID)
	if err != nil {
		fmt.Println("ON update users approved: ", err)
		http.Error(w, "Error updating approval status", http.StatusInternalServerError)
		return
	}

	// Remove all existing equipment permissions
	_, err = tx.Exec(context.Background(),
		"DELETE FROM user_equipment_permissions WHERE user_id = $1",
		data.UserID)
	if err != nil {
		fmt.Println("On deleting permissions:", err)
		http.Error(w, "Error updating equipment permissions", http.StatusInternalServerError)
		return
	}

	// Add new equipment permissions
	for _, equipID := range data.Equipment {
		_, err = tx.Exec(context.Background(),
			"INSERT INTO user_equipment_permissions (user_id, equipment_id) VALUES ($1, $2)",
			data.UserID, equipID)
		if err != nil {
			fmt.Println("On inserting permissions", err)
			continue
		}
	}

	// Commit transaction
	if err = tx.Commit(context.Background()); err != nil {
		fmt.Println("On commiting to db", err)
		http.Error(w, "Error committing changes", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getAllGroups() ([]Group, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT group_id, name
         FROM groups
         ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			continue
		}
		groups = append(groups, g)
	}
	return groups, nil
}
func requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := auth.MustSessionFromContext(r.Context())

		var isAdmin bool
		err := dbPool.QueryRow(context.Background(),
			"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
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
		"UPDATE users SET admin = $1 WHERE user_id = $2 AND COALESCE(deleted, false) = false",
		isAdmin, userID)

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin?success=Admin+status+updated", http.StatusSeeOther)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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
		idStr := r.FormValue("user_id")
		parsed, err := strconv.Atoi(idStr)
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

	cmdTag, err := dbPool.Exec(context.Background(),
		`UPDATE users
         SET deleted = true,
             is_approved = false
         WHERE user_id = $1
           AND COALESCE(deleted, false) = false`,
		userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if cmdTag.RowsAffected() == 0 {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if authManagerInstance != nil {
		authManagerInstance.RevokeUserSessions(userID)
	}

	if strings.Contains(contentType, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
		return
	}

	http.Redirect(w, r, "/admin?success=User+removed", http.StatusSeeOther)
}

func handleAddEquipment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get equipment name from form
	equipmentName := r.FormValue("name")
	if equipmentName == "" {
		http.Redirect(w, r, "/admin?error=Equipment+name+is+required", http.StatusSeeOther)
		return
	}

	// Insert new equipment
	_, err := dbPool.Exec(context.Background(),
		"INSERT INTO equipment (name) VALUES ($1)",
		equipmentName)

	if err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+add+equipment", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Equipment+added+successfully", http.StatusSeeOther)
}

func handleDeleteEquipment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract equipment ID from URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) != 4 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	equipmentID, err := strconv.Atoi(parts[3])
	if err != nil {
		http.Error(w, "Invalid equipment ID", http.StatusBadRequest)
		return
	}

	// Start a transaction
	tx, err := dbPool.Begin(context.Background())
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	// Delete related bookings first
	_, err = tx.Exec(context.Background(),
		"DELETE FROM bookings WHERE equipment_id = $1",
		equipmentID)
	if err != nil {
		http.Error(w, "Error deleting bookings", http.StatusInternalServerError)
		return
	}

	// Delete equipment permissions
	_, err = tx.Exec(context.Background(),
		"DELETE FROM user_equipment_permissions WHERE equipment_id = $1",
		equipmentID)
	if err != nil {
		http.Error(w, "Error deleting permissions", http.StatusInternalServerError)
		return
	}

	// Delete the equipment
	_, err = tx.Exec(context.Background(),
		"DELETE FROM equipment WHERE equipment_id = $1",
		equipmentID)
	if err != nil {
		http.Error(w, "Error deleting equipment", http.StatusInternalServerError)
		return
	}

	// Commit transaction
	if err = tx.Commit(context.Background()); err != nil {
		http.Error(w, "Error committing transaction", http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
}

func handleAddGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/admin?error=Group+name+is+required", http.StatusSeeOther)
		return
	}

	_, err := dbPool.Exec(context.Background(),
		"INSERT INTO groups (name) VALUES ($1) ON CONFLICT (name) DO NOTHING",
		name)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+add+group", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Group+added", http.StatusSeeOther)
}

func handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	groupID := parts[len(parts)-1]

	var groupName string
	if err := dbPool.QueryRow(context.Background(),
		"SELECT name FROM groups WHERE group_id = $1",
		groupID).Scan(&groupName); err != nil {
		http.Redirect(w, r, "/admin?error=Unknown+group", http.StatusSeeOther)
		return
	}

	_, err := dbPool.Exec(context.Background(),
		`UPDATE users SET "group" = NULL WHERE btrim("group") = $1`,
		groupName)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+unlink+users+from+group", http.StatusSeeOther)
		return
	}

	_, err = dbPool.Exec(context.Background(),
		"DELETE FROM groups WHERE group_id = $1",
		groupID)
	if err != nil {
		http.Redirect(w, r, "/admin?error=Failed+to+delete+group", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin?success=Group+removed", http.StatusSeeOther)
}

func handleEquipmentReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse equipment ID and date range
	equipmentID := r.URL.Query().Get("id")
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	// Query bookings
	rows, err := dbPool.Query(context.Background(), `
        SELECT 
            b.start_time,
            b.end_time,
            b.purpose,
            u.username,
            COALESCE(NULLIF(btrim(u."group"), ''), 'Unassigned') AS user_group,
            e.name as equipment_name
        FROM bookings b
        JOIN users u ON b.user_id = u.user_id
        JOIN equipment e ON b.equipment_id = e.equipment_id
        WHERE b.equipment_id = $1
        AND b.start_time >= $2
        AND b.end_time <= $3
        ORDER BY b.start_time ASC`,
		equipmentID, startDate, endDate)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Set up CSV writer
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=equipment_usage.csv")

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write headers
	headers := []string{"Start Time", "End Time", "Duration (hours)", "User", "Group", "Equipment", "Purpose"}
	if err := csvWriter.Write(headers); err != nil {
		http.Error(w, "Error writing CSV", http.StatusInternalServerError)
		return
	}

	// Write data rows
	for rows.Next() {
		var startTime, endTime time.Time
		var purpose, username, groupName, equipmentName string

		if err := rows.Scan(&startTime, &endTime, &purpose, &username, &groupName, &equipmentName); err != nil {
			continue
		}

		duration := endTime.Sub(startTime).Hours()

		row := []string{
			startTime.Format("2006-01-02 15:04"),
			endTime.Format("2006-01-02 15:04"),
			fmt.Sprintf("%.2f", duration),
			username,
			groupName,
			equipmentName,
			purpose,
		}

		if err := csvWriter.Write(row); err != nil {
			continue
		}
	}
}
