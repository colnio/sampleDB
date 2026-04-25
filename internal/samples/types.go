// Package samples provides the domain types and service for sample management,
// including attachments, sharing tokens, and fabrication steps.
package samples

import (
	"html/template"
	"time"

	"sampleDB/internal/activity"
)

// Sample represents a sample record in the database.
type Sample struct {
	ID              int
	Name            string
	Description     string
	Keywords        string
	Owner           string
	OwnerUserID     *int
	OwnerUsername   *string
	SamplePrep      string
	SamplePrepHTML  template.HTML
	Attachments     []Attachment
	CreatedAt       time.Time
	ShareToken      *string
	ShareVisibility string // 'private', 'public', 'authenticated'
	FirstImageID    *int
	Status          string // 'active', 'in_progress', 'archived'
	StepCount       int
}

// Attachment represents a file attached to a sample.
type Attachment struct {
	ID           int
	SampleID     int
	Address      string
	UploadedAt   time.Time
	OriginalName string
	ContentType  string
	IsImage      bool
}

// FabricationStep represents a single step in a sample's fabrication process.
type FabricationStep struct {
	ID             int
	SampleID       int
	StepOrder      int
	StepDate       *time.Time
	PersonUserID   *int
	PersonUsername *string
	Description    string
	EquipmentID    *int
	EquipmentName  *string
	EquipmentText  *string
	CreatedAt      time.Time
}

// User is a minimal user reference used for dropdowns and ownership attribution.
type User struct {
	ID       int
	Username string
}

// Equipment is a minimal equipment reference used in fabrication step forms.
type Equipment struct {
	ID   int
	Name string
}

// BasePageData holds session-derived fields shared by all page data structs.
type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

// MainPageData is the template data for the main sample list page.
type MainPageData struct {
	BasePageData
	Samples      []Sample
	Query        string
	DateFrom     string
	DateTo       string
	StatusFilter string // "", "active", "in_progress", "archived" (empty = all)
	OwnerFilter  string // "" (all) or a username
	View         string // "grid" (default) or "table"
	Users        []User // approved users for owner dropdown
	// Pre-built query strings (encoded, no leading "?") used by the Grid/Table
	// toggle links so they preserve every other active filter.
	GridQuery  string
	TableQuery string
}

// NewSamplePageData is the template data for the new-sample form.
type NewSamplePageData struct {
	BasePageData
	Users []User
}

// Statuses is the ordered set of sample statuses used for filter chips and
// select controls. Keeping it here makes the vocabulary authoritative and
// easy to render in templates.
var Statuses = []string{"active", "in_progress", "archived"}

// SampleDetailPageData is the template data for the sample detail page.
type SampleDetailPageData struct {
	BasePageData
	Sample      Sample
	Flash       string
	Error       string
	IsPartial   bool
	Users       []User
	Steps       []FabricationStep
	Equipment   []Equipment
	EditingStep int              // step_id currently in inline-edit mode (0 = none)
	ShareURL    string           // full URL for the share link, empty when not shared
	Activity    []activity.Entry // recent activity for this sample (newest first)
}

// SharedSamplePageData is the template data for the public shared sample view.
type SharedSamplePageData struct {
	BasePageData
	Sample          Sample
	IsAuthenticated bool
	ShareURL        string
	ShareToken      string
	Steps           []FabricationStep
}
