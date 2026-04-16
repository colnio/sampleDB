package booking

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// Handlers groups all HTTP handler methods for the booking domain.
type Handlers struct {
	svc      Service
	renderer *web.Renderer
	authMgr  *auth.Manager
	loc      *time.Location
}

// NewHandlers constructs a Handlers value.
func NewHandlers(svc Service, r *web.Renderer, a *auth.Manager, loc *time.Location) *Handlers {
	return &Handlers{svc: svc, renderer: r, authMgr: a, loc: loc}
}

// HandleBooking dispatches GET (calendar view) and POST (booking submission).
func (h *Handlers) HandleBooking(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.showCalendar(w, r)
	case http.MethodPost:
		h.submitBooking(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handlers) showCalendar(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	now := time.Now().In(h.loc)
	selectedDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, h.loc)

	if dateStr := r.URL.Query().Get("date"); dateStr != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", dateStr, h.loc); err == nil {
			selectedDate = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, h.loc)
		}
	}

	weekOffset := 0
	if offsetStr := r.URL.Query().Get("week"); offsetStr != "" {
		weekOffset, _ = strconv.Atoi(offsetStr)
		selectedDate = selectedDate.AddDate(0, 0, weekOffset*7)
	}

	weekStart := selectedDate.AddDate(0, 0, -int(selectedDate.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, h.loc)
	weekEnd := weekStart.AddDate(0, 0, 7)

	timeSlots := make([]time.Time, 96)
	hours := make([]time.Time, 24)
	baseTime := time.Date(selectedDate.Year(), selectedDate.Month(), selectedDate.Day(), 0, 0, 0, 0, h.loc)
	for i := range timeSlots {
		timeSlots[i] = baseTime.Add(time.Duration(i*15) * time.Minute)
		if i%4 == 0 {
			hours[i/4] = baseTime.Add(time.Duration(i/4) * time.Hour)
		}
	}

	equipment, err := h.svc.GetAllEquipment(r.Context())
	if err != nil {
		http.Error(w, "Error retrieving equipment", http.StatusInternalServerError)
		return
	}

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("booking: unable to determine admin status for user %d: %v", session.UserID, err)
	}

	hasPermission := make(map[int]bool)
	for _, eq := range equipment {
		permitted, err := h.svc.CheckUserPermission(r.Context(), session.UserID, eq.ID)
		if err != nil {
			continue
		}
		hasPermission[eq.ID] = permitted
	}

	bookings, err := h.svc.GetBookingsForWeek(r.Context(), weekStart, weekEnd)
	if err != nil {
		http.Error(w, "Error retrieving bookings", http.StatusInternalServerError)
		return
	}

	userBookings, err := h.svc.GetUserBookings(r.Context(), session.UserID)
	if err != nil {
		http.Error(w, "Error retrieving user bookings", http.StatusInternalServerError)
		return
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

	if err := h.renderer.Render(w, "booking", data); err != nil {
		http.Error(w, fmt.Sprintf("Template execution error: %v", err), http.StatusInternalServerError)
	}
}

func (h *Handlers) submitBooking(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	equipmentID, err := strconv.Atoi(r.FormValue("equipment_id"))
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+equipment", http.StatusSeeOther)
		return
	}

	startTime, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("start_time"), h.loc)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+start+time:+"+err.Error(), http.StatusSeeOther)
		return
	}

	endTime, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("end_time"), h.loc)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+end+time:+"+err.Error(), http.StatusSeeOther)
		return
	}

	if endTime.Before(startTime) {
		http.Redirect(w, r, "/booking?error=End+time+must+be+after+start+time", http.StatusSeeOther)
		return
	}

	hasConflict, err := h.svc.CheckConflict(r.Context(), equipmentID, startTime, endTime)
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

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Unable+to+verify+permissions", http.StatusSeeOther)
		return
	}

	if !isAdmin {
		permitted, err := h.svc.CheckUserPermission(r.Context(), session.UserID, equipmentID)
		if err != nil {
			http.Redirect(w, r, "/booking?error=Unable+to+verify+permissions", http.StatusSeeOther)
			return
		}
		if !permitted {
			http.Redirect(w, r, "/booking?error=You+do+not+have+access+to+this+equipment", http.StatusSeeOther)
			return
		}
	}

	err = h.svc.Create(r.Context(), CreateParams{
		EquipmentID: equipmentID,
		UserID:      session.UserID,
		StartTime:   startTime,
		EndTime:     endTime,
		Purpose:     purpose,
	})
	if err != nil {
		if errors.Is(err, web.ErrConflict) {
			http.Redirect(w, r, "/booking?error=Time+slot+already+booked", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/booking?error=Error+creating+booking:+"+err.Error(), http.StatusSeeOther)
		return
	}

	week := r.FormValue("week")
	if week != "" {
		http.Redirect(w, r, "/booking?week="+week+"&success=Booking+created+successfully", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/booking?success=Booking+created+successfully", http.StatusSeeOther)
	}
}

// DeleteBooking handles POST /booking/delete.
func (h *Handlers) DeleteBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	bookingID, err := strconv.Atoi(r.FormValue("booking_id"))
	if err != nil {
		http.Error(w, "Invalid booking ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.Delete(r.Context(), bookingID, session.UserID); err != nil {
		switch {
		case errors.Is(err, web.ErrNotFound):
			http.Error(w, "Booking not found", http.StatusNotFound)
		case errors.Is(err, web.ErrForbidden):
			http.Error(w, "Not authorized to delete this booking", http.StatusForbidden)
		default:
			http.Error(w, "Error deleting booking", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/booking?success=Booking+deleted", http.StatusSeeOther)
}

// GetBookings handles GET /api/bookings — returns JSON list of bookings for a date range.
func (h *Handlers) GetBookings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	bookings, err := h.svc.GetBookingsForWeek(r.Context(), start, end)
	if err != nil {
		http.Error(w, "Error retrieving bookings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookings)
}
