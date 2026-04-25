// Package booking provides the domain types, service, and HTTP handlers for the
// equipment booking feature (calendar view, create/delete bookings, permission checks).
package booking

import "time"

// Equipment represents a piece of lab equipment.
type Equipment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Status      string `json:"status"`
}

// Booking represents a single equipment reservation.
type Booking struct {
	ID          int       `json:"id"`
	EquipmentID int       `json:"equipment_id"`
	UserID      int       `json:"user_id"`
	Username    string    `json:"username"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Purpose     string    `json:"purpose"`
	CreatedAt   time.Time `json:"created_at"`
}

// BasePageData holds session-derived fields shared by all page templates.
type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

// BookingPageData is the template data for the booking calendar page.
type BookingPageData struct {
	BasePageData
	Equipment        []Equipment
	Bookings         []Booking
	HasPermission    map[int]bool // can SEE the lane (true for admins on all equipment)
	HasBookPermission map[int]bool // can actually BOOK (based on real group access)
	TimeSlots        []time.Time
	Hours            []time.Time
	WeekStart        time.Time
	WeekEnd          time.Time
	WeekOffset       int
	SelectedDate     time.Time
	Error            string
	Success          string
}

// CreateParams holds the fields for creating a new booking.
type CreateParams struct {
	EquipmentID int
	UserID      int
	StartTime   time.Time
	EndTime     time.Time
	Purpose     string
}
