// Package admin provides the domain types, service, and HTTP handlers for the
// admin section (user management, group management, equipment management).
package admin

import "time"

// Equipment represents a piece of lab equipment.
type Equipment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
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
	Approved        bool        `json:"approved"`
	Admin           bool        `json:"admin"`
	GroupName       string      `json:"group_name"`
	CreatedAt       string      `json:"created_at"`
	EquipmentAccess []Equipment `json:"equipment_access"`
}

// BasePageData holds session-derived fields shared by all page templates.
type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

// AdminPageData is the template data for the admin page.
type AdminPageData struct {
	BasePageData
	Users     []UserAccess
	Equipment []Equipment
	Groups    []Group
	Error     string
	Success   string
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

// UpdateAccessParams holds the fields for updating a user's access.
type UpdateAccessParams struct {
	UserID    int
	Approved  bool
	GroupName *string
	Equipment []int
}
