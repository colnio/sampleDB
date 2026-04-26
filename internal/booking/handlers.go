package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"sampleDB/internal/activity"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// Handlers groups all HTTP handler methods for the booking domain.
type Handlers struct {
	svc      Service
	renderer *web.Renderer
	authMgr  *auth.Manager
	loc      *time.Location
	// Activity is a best-effort write-path. A nil Logger is safe.
	Activity *activity.Logger
}

// NewHandlers constructs a Handlers value.
func NewHandlers(svc Service, r *web.Renderer, a *auth.Manager, loc *time.Location) *Handlers {
	return &Handlers{svc: svc, renderer: r, authMgr: a, loc: loc}
}

// logActivity writes a best-effort activity_log row.
func (h *Handlers) logActivity(ctx context.Context, session auth.Session, verb activity.Verb, targetType activity.TargetType, targetID int, label string) {
	if h == nil || h.Activity == nil {
		return
	}
	actor := activity.Actor{ID: session.UserID, Username: session.Username}
	if err := h.Activity.Log(ctx, actor, verb, targetType, targetID, label); err != nil {
		log.Printf("booking: activity log (%s/%s) failed: %v", verb, targetType, err)
	}
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
		if n, err := strconv.Atoi(offsetStr); err == nil {
			if n < -52 {
				n = -52
			} else if n > 52 {
				n = 52
			}
			weekOffset = n
			selectedDate = selectedDate.AddDate(0, 0, weekOffset*7)
		}
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
		http.Error(w, "Error checking permissions", http.StatusInternalServerError)
		return
	}

	hasBookPermission := make(map[int]bool) // can the current user create a booking?
	hasPermission := make(map[int]bool)      // can the current user see the lane?

	for _, eq := range equipment {
		permitted, err := h.svc.CheckUserPermission(r.Context(), session.UserID, eq.ID)
		if err != nil {
			log.Printf("booking: CheckUserPermission error for equipment %d: %v", eq.ID, err)
			http.Error(w, "Error checking permissions", http.StatusInternalServerError)
			return
		}

		// Booking access depends on status:
		//   offline      → nobody can book (equipment is down)
		//   maintenance  → admin + group access only (scheduled maintenance)
		//   online       → normal group-access check
		switch eq.Status {
		case "offline":
			hasBookPermission[eq.ID] = false
		case "maintenance":
			hasBookPermission[eq.ID] = isAdmin && permitted
		default:
			hasBookPermission[eq.ID] = permitted
		}

		// Lane visibility: maintenance/offline equipment is shown to everyone so
		// users can see its status. Online equipment is only shown to users who
		// have group access (or are admins).
		switch eq.Status {
		case "offline", "maintenance":
			hasPermission[eq.ID] = true
		default:
			hasPermission[eq.ID] = isAdmin || permitted
		}
	}

	bookings, err := h.svc.GetBookingsForWeek(r.Context(), weekStart, weekEnd)
	if err != nil {
		http.Error(w, "Error retrieving bookings", http.StatusInternalServerError)
		return
	}

	data := BookingPageData{
		BasePageData:      BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin, CSRFToken: session.CSRFToken},
		Equipment:         equipment,
		Bookings:          bookings,
		HasPermission:     hasPermission,
		HasBookPermission: hasBookPermission,
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
		log.Printf("booking: template render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handlers) submitBooking(w http.ResponseWriter, r *http.Request) {
	session := auth.MustSessionFromContext(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
		http.Redirect(w, r, "/booking?error=Invalid+start+time+format", http.StatusSeeOther)
		return
	}

	endTime, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("end_time"), h.loc)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+end+time+format", http.StatusSeeOther)
		return
	}

	if !endTime.After(startTime) {
		http.Redirect(w, r, "/booking?error=End+time+must+be+after+start+time", http.StatusSeeOther)
		return
	}

	const maxBookingDuration = 30 * 24 * time.Hour
	if endTime.Sub(startTime) > maxBookingDuration {
		http.Redirect(w, r, "/booking?error=Booking+duration+cannot+exceed+30+days", http.StatusSeeOther)
		return
	}

	purpose := strings.TrimSpace(r.FormValue("purpose"))
	if purpose == "" {
		http.Redirect(w, r, "/booking?error=Purpose+is+required", http.StatusSeeOther)
		return
	}
	if len(purpose) > 500 {
		http.Redirect(w, r, "/booking?error=Purpose+must+be+500+characters+or+fewer", http.StatusSeeOther)
		return
	}

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Unable+to+verify+permissions", http.StatusSeeOther)
		return
	}

	eqStatus, err := h.svc.GetEquipmentStatus(r.Context(), equipmentID)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+equipment", http.StatusSeeOther)
		return
	}
	if eqStatus == "offline" {
		http.Redirect(w, r, "/booking?error=Equipment+is+currently+offline", http.StatusSeeOther)
		return
	}
	if eqStatus == "maintenance" && !isAdmin {
		http.Redirect(w, r, "/booking?error=Equipment+is+under+maintenance", http.StatusSeeOther)
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

	hasConflict, err := h.svc.CheckConflict(r.Context(), equipmentID, startTime, endTime)
	if err != nil {
		http.Redirect(w, r, "/booking?error=Unable+to+check+availability", http.StatusSeeOther)
		return
	}
	if hasConflict {
		http.Redirect(w, r, "/booking?error=Time+slot+already+booked", http.StatusSeeOther)
		return
	}

	bookingID, err := h.svc.Create(r.Context(), CreateParams{
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
		log.Printf("booking: error creating booking for user %d: %v", session.UserID, err)
		http.Redirect(w, r, "/booking?error=Error+creating+booking", http.StatusSeeOther)
		return
	}
	eqName, _ := h.svc.GetEquipmentName(r.Context(), equipmentID)
	h.logActivity(r.Context(), session, activity.Verb("booked"), activity.TargetType("booking"), bookingID, eqName)

	week := r.FormValue("week")
	if week != "" {
		if n, err := strconv.Atoi(week); err == nil {
			http.Redirect(w, r, fmt.Sprintf("/booking?week=%d&success=Booking+created+successfully", n), http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/booking?success=Booking+created+successfully", http.StatusSeeOther)
}

// DeleteBooking handles POST /booking/delete.
func (h *Handlers) DeleteBooking(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+request", http.StatusSeeOther)
		return
	}

	session := auth.MustSessionFromContext(r.Context())
	bookingID, err := strconv.Atoi(r.FormValue("booking_id"))
	if err != nil {
		http.Redirect(w, r, "/booking?error=Invalid+booking", http.StatusSeeOther)
		return
	}

	isAdmin, err := h.svc.IsAdmin(r.Context(), session.UserID)
	if err != nil {
		log.Printf("booking: unable to determine admin status for user %d: %v", session.UserID, err)
		http.Error(w, "Error checking permissions", http.StatusInternalServerError)
		return
	}

	// If an admin is deleting someone else's booking, require password confirmation.
	if isAdmin {
		existing, err := h.svc.GetBooking(r.Context(), bookingID)
		if err != nil {
			if errors.Is(err, web.ErrNotFound) {
				http.Redirect(w, r, "/booking?error=Booking+not+found", http.StatusSeeOther)
			} else {
				log.Printf("booking: GetBooking %d: %v", bookingID, err)
				http.Redirect(w, r, "/booking?error=Could+not+load+booking", http.StatusSeeOther)
			}
			return
		}
		if existing.UserID != session.UserID {
			adminPwd := r.FormValue("admin_password")
			if adminPwd == "" {
				http.Redirect(w, r, "/booking?error=Password+required+to+delete+another+user%27s+booking", http.StatusSeeOther)
				return
			}
			ok, err := h.svc.VerifyUserPassword(r.Context(), session.UserID, adminPwd)
			if err != nil {
				log.Printf("booking: password verify error for admin %d: %v", session.UserID, err)
				http.Redirect(w, r, "/booking?error=Internal+error+verifying+password", http.StatusSeeOther)
				return
			}
			if !ok {
				http.Redirect(w, r, "/booking?error=Incorrect+password", http.StatusSeeOther)
				return
			}
		}
	}

	if err := h.svc.Delete(r.Context(), bookingID, session.UserID, isAdmin); err != nil {
		switch {
		case errors.Is(err, web.ErrNotFound):
			http.Redirect(w, r, "/booking?error=Booking+not+found", http.StatusSeeOther)
		case errors.Is(err, web.ErrForbidden):
			http.Redirect(w, r, "/booking?error=Not+authorized+to+delete+this+booking", http.StatusSeeOther)
		default:
			log.Printf("booking: delete error: %v", err)
			http.Redirect(w, r, "/booking?error=Error+deleting+booking", http.StatusSeeOther)
		}
		return
	}
	h.logActivity(r.Context(), session, activity.Verb("deleted"), activity.TargetType("booking"), bookingID, "")

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

	start, err := time.ParseInLocation("2006-01-02", startStr, h.loc)
	if err != nil {
		http.Error(w, "Invalid start date", http.StatusBadRequest)
		return
	}

	end, err := time.ParseInLocation("2006-01-02", endStr, h.loc)
	if err != nil {
		http.Error(w, "Invalid end date", http.StatusBadRequest)
		return
	}

	if !end.After(start) {
		http.Error(w, "end must be after start", http.StatusBadRequest)
		return
	}
	if end.Sub(start) > 90*24*time.Hour {
		http.Error(w, "date range too large (max 90 days)", http.StatusBadRequest)
		return
	}

	bookings, err := h.svc.GetBookingsForWeek(r.Context(), start, end)
	if err != nil {
		http.Error(w, "Error retrieving bookings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(bookings); err != nil {
		log.Printf("booking: GetBookings encode error: %v", err)
	}
}
