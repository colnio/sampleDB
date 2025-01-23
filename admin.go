package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
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

func getBasePageData(session Session) (BasePageData, error) {
	var isAdmin bool
	err := dbPool.QueryRow(context.Background(),
		"SELECT admin FROM users WHERE user_id = $1",
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
	session := r.Context().Value("user").(Session)

	// Get base page data
	baseData, err := getBasePageData(session)
	if err != nil {
		http.Error(w, "Error getting user data", http.StatusInternalServerError)
		return
	}

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
            COALESCE(
                array_agg(uep.equipment_id) FILTER (WHERE uep.equipment_id IS NOT NULL),
                '{}'::int[]
            ) as equipment_ids
        FROM users u
        LEFT JOIN user_equipment_permissions uep ON u.user_id = uep.user_id
        GROUP BY u.user_id, u.username, u.is_approved, u.admin
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
			fmt.Println(err)
			continue
		}

		// Map equipment permissions
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
		Error:        r.URL.Query().Get("error"),
		Success:      r.URL.Query().Get("success"),
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

	// Update approved status
	_, err = tx.Exec(context.Background(),
		"UPDATE users SET is_approved = $1 WHERE user_id = $2",
		data.Approved, data.UserID)
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
	headers := []string{"Start Time", "End Time", "Duration (hours)", "User", "Equipment", "Purpose"}
	if err := csvWriter.Write(headers); err != nil {
		http.Error(w, "Error writing CSV", http.StatusInternalServerError)
		return
	}

	// Write data rows
	for rows.Next() {
		var startTime, endTime time.Time
		var purpose, username, equipmentName string

		if err := rows.Scan(&startTime, &endTime, &purpose, &username, &equipmentName); err != nil {
			continue
		}

		duration := endTime.Sub(startTime).Hours()

		row := []string{
			startTime.Format("2006-01-02 15:04"),
			endTime.Format("2006-01-02 15:04"),
			fmt.Sprintf("%.2f", duration),
			username,
			equipmentName,
			purpose,
		}

		if err := csvWriter.Write(row); err != nil {
			continue
		}
	}
}
