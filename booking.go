package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"sampleDB/internal/auth"
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
	Hours         []time.Time
	WeekStart     time.Time
	WeekEnd       time.Time
	WeekOffset    int
	SelectedDate  time.Time
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
		"toJSON": func(value interface{}) template.JS {
			data, err := json.Marshal(value)
			if err != nil || string(data) == "null" {
				return template.JS("[]")
			}
			return template.JS(data)
		},
		// Date formatting
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006")
		},
		"formatDateShort": func(t time.Time) string {
			return t.Format("Jan 2")
		},
		"formatDateNumeric": func(t time.Time) string {
			return t.Format("01/02/2006")
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
		"addDays": func(t time.Time, days int) time.Time {
			return t.AddDate(0, 0, days)
		},
	}

	// Create new template with function map
	tmpl := template.New("").Funcs(funcMap)

	// Always include base and header templates
	baseTemplates := []string{"templates/base.html", "templates/header.html"}
	resolved := make([]string, 0, len(files)+len(baseTemplates))
	for _, f := range baseTemplates {
		resolved = append(resolved, resolveTemplatePath(f))
	}
	for _, f := range files {
		resolved = append(resolved, resolveTemplatePath(f))
	}

	// Parse all template files
	return tmpl.ParseFiles(resolved...)
}

func showBookingCalendar(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	now := time.Now().In(loc)
	selectedDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	if dateStr := r.URL.Query().Get("date"); dateStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", dateStr, loc); err == nil {
			selectedDate = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
		}
	}

	// Parse week offset
	weekOffset := 0
	if offsetStr := r.URL.Query().Get("week"); offsetStr != "" {
		weekOffset, _ = strconv.Atoi(offsetStr)
		selectedDate = selectedDate.AddDate(0, 0, weekOffset*7)
	}

	// Calculate week boundaries using local timezone around selected date
	weekStart := selectedDate.AddDate(0, 0, -int(selectedDate.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, loc)
	weekEnd := weekStart.AddDate(0, 0, 7)

	// Generate hourly time slots in local timezone
	timeSlots := make([]time.Time, 96) // 15-minute slots
	hours := make([]time.Time, 24)
	baseTime := time.Date(selectedDate.Year(), selectedDate.Month(), selectedDate.Day(), 0, 0, 0, 0, loc)
	for i := range timeSlots {
		timeSlots[i] = baseTime.Add(time.Duration(i*15) * time.Minute)
		if i%4 == 0 {
			hours[i/4] = baseTime.Add(time.Duration(i/4) * time.Hour)
		}
	}

	// Get equipment and other data...
	equipment, err := getAllEquipment()
	if err != nil {
		http.Error(w, "Error retrieving equipment", http.StatusInternalServerError)
		return
	}

	isAdmin, err := isUserAdmin(session.UserID)
	if err != nil {
		log.Printf("booking: unable to determine admin status for user %d: %v", session.UserID, err)
	}

	hasPermission := make(map[int]bool)
	for _, eq := range equipment {
		// if isAdmin {
		// 	hasPermission[eq.ID] = true
		// 	continue
		// }
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
	if len(bookings) == 0 {
		bookings = make([]Booking, 0)
	}

	userBookings, err := getUserBookings(session.UserID)
	if err != nil {
		http.Error(w, "Error retrieving user bookings", http.StatusInternalServerError)
		return
	}
	if len(userBookings) == 0 {
		userBookings = make([]Booking, 0)
	}

	data := BookingPageData{
		BasePageData:  BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		Equipment:     equipment,
		Bookings:      bookings,
		UserBookings:  userBookings,
		HasPermission: hasPermission,
		TimeSlots:     timeSlots,
		Hours:         hours,
		WeekStart:     weekStart,
		WeekEnd:       weekEnd,
		WeekOffset:    weekOffset,
		SelectedDate:  selectedDate,
		Error:         r.URL.Query().Get("error"),
		Success:       r.URL.Query().Get("success"),
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
	session := auth.MustSessionFromContext(r.Context())

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

	isAdmin, err := isUserAdmin(session.UserID)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Unable+to+verify+permissions", http.StatusSeeOther)
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

	if !isAdmin {
		permitted, err := checkUserPermission(session.UserID, equipmentID)
		if err != nil {
			http.Redirect(w, r, "/booking?error=Unable+to+verify+permissions", http.StatusSeeOther)
			return
		}
		if !permitted {
			http.Redirect(w, r, "/booking?error=You+do+not+have+access+to+this+equipment", http.StatusSeeOther)
			return
		}
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

func isUserAdmin(userID int) (bool, error) {
	var isAdmin bool
	err := dbPool.QueryRow(context.Background(),
		`SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false`,
		userID).Scan(&isAdmin)
	return isAdmin, err
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
	if len(bookings) == 0 {
		return []Booking{}, nil
	}
	return bookings, nil
}

// Add a new handler for deleting bookings
func handleDeleteBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
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
	if len(bookings) == 0 {
		return []Booking{}, nil
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
