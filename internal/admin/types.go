// Package admin provides the domain types, service, and HTTP handlers for the
// admin section (user management, group management, equipment management).
package admin

import (
	"fmt"
	"time"
)

// Notification is an admin-posted banner shown to all users.
type Notification struct {
	ID        int64     `json:"id"`
	Message   string    `json:"message"`
	Level     string    `json:"level"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// Equipment represents a piece of lab equipment.
type Equipment struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	Status      string   `json:"status"`
	Groups      []string `json:"groups"`
}

// Group represents a user group.
type Group struct {
	ID   int
	Name string
}

// UserAccess holds a user's profile and equipment permissions for the admin page.
type UserAccess struct {
	UserID          int         `json:"user_id"`
	Username        string      `json:"username"`
	Email           string      `json:"email"`
	Approved        bool        `json:"approved"`
	Admin           bool        `json:"admin"`
	GroupName       string      `json:"group_name"`
	CreatedAt       string      `json:"created_at"`
	EquipmentAccess []Equipment `json:"equipment_access"`
	// AFMOutsideHoursAllowed lets the user book the AFM outside working hours.
	AFMOutsideHoursAllowed bool `json:"afm_outside_hours_allowed"`
}

// BasePageData holds session-derived fields shared by all page templates.
type BasePageData struct {
	Username  string
	UserID    int
	IsAdmin   bool
	CSRFToken string
}

// AdminPageData is the template data for the admin page.
type AdminPageData struct {
	BasePageData
	Users         []UserAccess
	Equipment     []Equipment
	Groups        []Group
	Notifications []Notification
	WorkingHours  WorkingHours
	Exceptions    []WorkingHoursException
	AFMEquipment  []string // names of the equipment the working hours apply to
	Error         string
	Success       string
}

// WorkingHours is the regular AFM working schedule.
type WorkingHours struct {
	StartMinute int     // minutes from midnight
	EndMinute   int     // minutes from midnight
	Weekdays    []int32 // 0 = Sunday
}

// Start returns the opening time as HH:MM.
func (w WorkingHours) Start() string { return formatMinute(w.StartMinute) }

// End returns the closing time as HH:MM.
func (w WorkingHours) End() string { return formatMinute(w.EndMinute) }

// WeekdayOption is one day checkbox in the working hours form.
type WeekdayOption struct {
	Value   int
	Label   string
	Checked bool
}

// DayOptions lists the days Monday first, marking the working ones.
func (w WorkingHours) DayOptions() []WeekdayOption {
	options := make([]WeekdayOption, 0, 7)
	for _, wd := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		opt := WeekdayOption{Value: int(wd), Label: wd.String()[:3]}
		for _, d := range w.Weekdays {
			if d == int32(wd) {
				opt.Checked = true
			}
		}
		options = append(options, opt)
	}
	return options
}

// WorkingHoursException overrides the schedule for one date, such as a holiday.
// Nil times mean there are no working hours that day.
type WorkingHoursException struct {
	Date        string // YYYY-MM-DD
	StartMinute *int
	EndMinute   *int
	Note        string
}

// Hours describes the exception for display.
func (e WorkingHoursException) Hours() string {
	if e.StartMinute == nil || e.EndMinute == nil {
		return "No working hours"
	}
	return formatMinute(*e.StartMinute) + "–" + formatMinute(*e.EndMinute)
}

func formatMinute(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// BookingRecord is a single row in the equipment usage CSV report.
type BookingRecord struct {
	StartTime     time.Time
	EndTime       time.Time
	Purpose       string
	Username      string
	GroupName     string
	EquipmentName string
}

// APIUser is the JSON representation of a user in the /api/v1/users response.
type APIUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	GroupName string `json:"group"`
}

// APIEquipment is the JSON representation of equipment in the /api/v1/equipment response.
type APIEquipment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

// UpdateAccessParams holds the fields for updating a user's access.
type UpdateAccessParams struct {
	UserID    int
	Approved  bool
	GroupName *string
	Equipment []int
	// AFMOutsideHoursAllowed is left unchanged when nil.
	AFMOutsideHoursAllowed *bool
}
