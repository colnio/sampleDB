// Package dashboard provides the aggregated landing page that shows KPIs,
// recent samples, an activity feed, and the day's bookings.
package dashboard

import (
	"time"

	"sampleDB/internal/activity"
)

// KPIs is the set of counts displayed in the top-of-page card grid.
type KPIs struct {
	TotalSamples     int
	ActiveBookings   int
	WikiArticles     int
	PendingApprovals int
}

// RecentSample is a minimal projection used for the "Recent samples" list.
type RecentSample struct {
	ID            int
	Name          string
	Description   string
	Owner         string
	OwnerUsername *string
	Status        string
	CreatedAt     time.Time
	FirstImageID  *int
}

// DisplayOwner returns the best available owner string for the card.
func (r RecentSample) DisplayOwner() string {
	if r.OwnerUsername != nil && *r.OwnerUsername != "" {
		return *r.OwnerUsername
	}
	if r.Owner != "" {
		return r.Owner
	}
	return "Unassigned"
}

// TodayBooking is a single row in the "Today's bookings" timeline.
type TodayBooking struct {
	ID            int
	EquipmentID   int
	EquipmentName string
	UserID        int
	Username      string
	StartTime     time.Time
	EndTime       time.Time
	Purpose       string
}

// BasePageData mirrors the session-derived fields used across all templates.
type BasePageData struct {
	Username  string
	UserID    int
	IsAdmin   bool
	CSRFToken string
}

// PageData is the template context for /dashboard.
type PageData struct {
	BasePageData
	KPIs          KPIs
	RecentSamples []RecentSample
	Activity      []activity.Entry
	TodayBookings []TodayBooking
	Now           time.Time
}
