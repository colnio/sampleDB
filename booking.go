package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"
)

type Equipment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

type Booking struct {
	ID          int       `json:"id"`
	EquipmentID int       `json:"equipment_id"`
	UserID      int       `json:"user_id"`
	Username    string    `json:"username"` // Add this field
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Purpose     string    `json:"purpose"`
	CreatedAt   time.Time `json:"created_at"`
}

type BookingPageData struct {
	BasePageData
	Equipment     []Equipment
	Bookings      []Booking
	UserBookings  []Booking
	HasPermission map[int]bool
	TimeSlots     []time.Time
	WeekStart     time.Time
	WeekEnd       time.Time
	WeekOffset    int
	Error         string
	Success       string
}

func handleBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		showBookingCalendar(w, r)
		return
	}

	if r.Method == http.MethodPost {
		handleBookingSubmission(w, r)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func parseTemplatesBooking(files ...string) (*template.Template, error) {
	// Create a new template with base name and include all functions
	funcMap := template.FuncMap{
		// Date formatting
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006")
		},
		"formatDateShort": func(t time.Time) string {
			return t.Format("Jan 2")
		},
		"formatDateISO": func(t time.Time) string {
			return t.Format("2006-01-02")
		},
		"formatDayName": func(t time.Time) string {
			return t.Format("Monday")
		},

		// Time formatting
		"formatTime": func(t time.Time) string {
			return t.Format("3:04 PM")
		},
		"formatTime24": func(t time.Time) string {
			return t.Format("15:04")
		},
		"formatDateTime": func(t time.Time) string {
			return t.Format("Jan 2, 15:04")
		},

		// Math operations
		"add": func(a, b int) int {
			return a + b
		},
		"subtract": func(a, b int) int {
			return a - b
		},

		// Utility functions
		"iterate": func(start, end int) []int {
			var result []int
			for i := start; i < end; i++ {
				result = append(result, i)
			}
			return result
		},
		"combineDatetime": func(date, timeSlot time.Time) time.Time {
			return time.Date(
				date.Year(),
				date.Month(),
				date.Day(),
				timeSlot.Hour(),
				timeSlot.Minute(),
				0, 0,
				loc, // Use local timezone
			)
		},

		"formatDateTimeISO": func(t time.Time) string {
			return t.Format("2006-01-02T15:04")
		},
	}

	// Create new template with function map
	tmpl := template.New("").Funcs(funcMap)

	// Always include base and header templates
	files = append([]string{
		"templates/base.html",
		"templates/header.html",
	}, files...)

	// Parse all template files
	return tmpl.ParseFiles(files...)
}

func showBookingCalendar(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)

	// Parse week offset
	weekOffset := 0
	if offsetStr := r.URL.Query().Get("week"); offsetStr != "" {
		weekOffset, _ = strconv.Atoi(offsetStr)
	}

	// Calculate week boundaries using local timezone
	now := time.Now().In(loc)
	weekStart := now.AddDate(0, 0, weekOffset*7-int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)

	// Generate time slots in local timezone
	timeSlots := make([]time.Time, 24) // 8 AM to 5 PM
	baseTime := time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, loc)
	for i := range timeSlots {
		timeSlots[i] = baseTime.Add(time.Duration(i) * time.Hour)
	}

	// Get equipment and other data...
	equipment, err := getAllEquipment()
	if err != nil {
		http.Error(w, "Error retrieving equipment", http.StatusInternalServerError)
		return
	}

	hasPermission := make(map[int]bool)
	for _, eq := range equipment {
		permitted, err := checkUserPermission(session.UserID, eq.ID)
		if err != nil {
			continue
		}
		hasPermission[eq.ID] = permitted
	}

	bookings, err := getBookingsForWeek(weekStart, weekEnd)
	if err != nil {
		http.Error(w, "Error retrieving bookings", http.StatusInternalServerError)
		return
	}

	userBookings, err := getUserBookings(session.UserID)
	if err != nil {
		http.Error(w, "Error retrieving user bookings", http.StatusInternalServerError)
		return
	}

	rows := dbPool.QueryRow(context.Background(), `SELECT admin
	FROM users
	WHERE username = $1`, session.Username)

	data := BookingPageData{
		BasePageData:  BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: false},
		Equipment:     equipment,
		Bookings:      bookings,
		UserBookings:  userBookings,
		HasPermission: hasPermission,
		TimeSlots:     timeSlots,
		WeekStart:     weekStart,
		WeekEnd:       weekEnd,
		WeekOffset:    weekOffset,
		Error:         r.URL.Query().Get("error"),
		Success:       r.URL.Query().Get("success"),
	}

	err = rows.Scan(&data.BasePageData.IsAdmin)
	if err != nil {
		fmt.Println(err)
	}

	tmpl, err := parseTemplatesBooking("templates/booking.html")
	if err != nil {
		http.Error(w, "Error loading template", http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, "base", data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template execution error: %v", err), http.StatusInternalServerError)
	}
}

func handleBookingSubmission(w http.ResponseWriter, r *http.Request) {
	session := r.Context().Value("user").(Session)

	// Parse form
	err := r.ParseForm()
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	// Parse equipment ID
	equipmentID, err := strconv.Atoi(r.FormValue("equipment_id"))
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+equipment", http.StatusSeeOther)
		return
	}

	// Parse dates and times using global location
	startTime, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("start_time"), loc)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+start+time:+"+err.Error(), http.StatusSeeOther)
		return
	}

	endTime, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("end_time"), loc)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+end+time:+"+err.Error(), http.StatusSeeOther)
		return
	}

	// Basic validation
	if endTime.Before(startTime) {
		http.Redirect(w, r, "/booking?error=End+time+must+be+after+start+time", http.StatusSeeOther)
		return
	}

	// if startTime.Before(time.Now()) {
	// 	http.Redirect(w, r, "/booking?error=Cannot+book+in+the+past", http.StatusSeeOther)
	// 	return
	// }

	// Check for conflicts
	hasConflict, err := checkBookingConflict(equipmentID, startTime, endTime)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Error+checking+conflicts", http.StatusSeeOther)
		return
	}
	if hasConflict {
		http.Redirect(w, r, "/booking?error=Time+slot+already+booked", http.StatusSeeOther)
		return
	}

	purpose := r.FormValue("purpose")
	if purpose == "" {
		http.Redirect(w, r, "/booking?error=Purpose+is+required", http.StatusSeeOther)
		return
	}

	// Create booking
	_, err = dbPool.Exec(context.Background(),
		`INSERT INTO bookings (equipment_id, user_id, start_time, end_time, purpose) 
         VALUES ($1, $2, $3, $4, $5)`,
		equipmentID, session.UserID, startTime, endTime, purpose)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Error+creating+booking:+"+err.Error(), http.StatusSeeOther)
		return
	}

	// Redirect back to the same week view
	week := r.FormValue("week")
	if week != "" {
		http.Redirect(w, r, "/booking?week="+week+"&success=Booking+created+successfully", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/booking?success=Booking+created+successfully", http.StatusSeeOther)
	}
}

// Helper functions

func getAllEquipment() ([]Equipment, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT equipment_id, name, coalesce(description, 'N/A'), coalesce(location, 'N/A') 
         FROM equipment 
         ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equipment []Equipment
	for rows.Next() {
		var eq Equipment
		err := rows.Scan(&eq.ID, &eq.Name, &eq.Description, &eq.Location)
		if err != nil {
			continue
		}
		equipment = append(equipment, eq)
	}
	return equipment, nil
}

func checkUserPermission(userID, equipmentID int) (bool, error) {
	var exists bool
	err := dbPool.QueryRow(context.Background(),
		`SELECT EXISTS(
            SELECT 1 FROM user_equipment_permissions 
            WHERE user_id = $1 AND equipment_id = $2
        )`, userID, equipmentID).Scan(&exists)
	return exists, err
}

func getBookingsForWeek(start, end time.Time) ([]Booking, error) {
	// Force timezone to local for query
	start = start.In(loc)
	end = end.In(loc)

	rows, err := dbPool.Query(context.Background(),
		`SELECT b.booking_id, b.equipment_id, b.user_id, u.username, 
                b.start_time AT TIME ZONE 'UTC', 
                b.end_time AT TIME ZONE 'UTC', 
                b.purpose, b.created_at 
         FROM bookings b 
         JOIN users u ON b.user_id = u.user_id 
         WHERE b.start_time >= $1 AND b.end_time <= $2 
         ORDER BY b.start_time`,
		start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		err := rows.Scan(&b.ID, &b.EquipmentID, &b.UserID, &b.Username,
			&b.StartTime, &b.EndTime, &b.Purpose, &b.CreatedAt)
		if err != nil {
			continue
		}
		// Ensure times are in local timezone
		b.StartTime = b.StartTime.In(loc)
		b.EndTime = b.EndTime.In(loc)
		bookings = append(bookings, b)
	}
	return bookings, nil
}

// Add a new handler for deleting bookings
func handleDeleteBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := r.Context().Value("user").(Session)
	bookingID := r.FormValue("booking_id")

	// Verify the booking belongs to the user
	var userID int
	err := dbPool.QueryRow(context.Background(),
		"SELECT user_id FROM bookings WHERE booking_id = $1", bookingID).Scan(&userID)
	if err != nil {
		http.Error(w, "Booking not found", http.StatusNotFound)
		return
	}

	if userID != session.UserID {
		http.Error(w, "Not authorized to delete this booking", http.StatusForbidden)
		return
	}

	// Delete the booking
	_, err = dbPool.Exec(context.Background(),
		"DELETE FROM bookings WHERE booking_id = $1", bookingID)
	if err != nil {
		http.Error(w, "Error deleting booking", http.StatusInternalServerError)
		return
	}

	// Redirect back to the booking page
	http.Redirect(w, r, "/booking?success=Booking+deleted", http.StatusSeeOther)
}

func getUserBookings(userID int) ([]Booking, error) {
	rows, err := dbPool.Query(context.Background(),
		`SELECT booking_id, equipment_id, user_id, start_time, end_time, purpose, created_at 
         FROM bookings 
         WHERE user_id = $1 AND end_time >= NOW() 
         ORDER BY start_time`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		err := rows.Scan(&b.ID, &b.EquipmentID, &b.UserID, &b.StartTime, &b.EndTime, &b.Purpose, &b.CreatedAt)
		if err != nil {
			continue
		}
		bookings = append(bookings, b)
	}
	return bookings, nil
}

func checkBookingConflict(equipmentID int, start, end time.Time) (bool, error) {
	var exists bool
	err := dbPool.QueryRow(context.Background(),
		`SELECT EXISTS(
            SELECT 1 FROM bookings 
            WHERE equipment_id = $1 
            AND (
                (start_time, end_time) OVERLAPS ($2, $3)
            )
        )`, equipmentID, start, end).Scan(&exists)
	return exists, err
}

// AJAX handlers for dynamic updates
func handleGetBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse date range from query parameters
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		http.Error(w, "Invalid start date", http.StatusBadRequest)
		return
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		http.Error(w, "Invalid end date", http.StatusBadRequest)
		return
	}

	bookings, err := getBookingsForWeek(start, end)
	if err != nil {
		http.Error(w, "Error retrieving bookings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookings)
}
