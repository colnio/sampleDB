package dashboard

import (
	"context"
	"log"
	"net/http"
	"time"

	"sampleDB/internal/activity"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

// adminChecker is a minimal interface that lets the handler detect whether the
// current user is an admin (so the sidebar renders the Admin group). The
// samples service already provides this lookup and is injected here so the
// dashboard package has no direct DB dependency beyond the dashboard queries.
type adminChecker interface {
	IsAdmin(ctx context.Context, userID int) (bool, error)
}

// activityReader is the subset of *activity.Logger that the dashboard calls.
// Using an interface keeps unit tests lightweight — they substitute a fake and
// don't need a real pool.
type activityReader interface {
	Recent(ctx context.Context, limit int) ([]activity.Entry, error)
}

// Handlers renders the /dashboard page.
type Handlers struct {
	svc      Service
	renderer *web.Renderer
	loc      *time.Location
	admin    adminChecker
	activity activityReader
	// Now returns the current time; tests override this to get deterministic
	// "today" values.
	Now func() time.Time
}

// NewHandlers wires the dashboard handler dependencies.
func NewHandlers(svc Service, renderer *web.Renderer, admin adminChecker, act activityReader, loc *time.Location) *Handlers {
	if loc == nil {
		loc = time.UTC
	}
	return &Handlers{
		svc:      svc,
		renderer: renderer,
		loc:      loc,
		admin:    admin,
		activity: act,
		Now:      time.Now,
	}
}

// DashboardPage serves GET /dashboard.
func (h *Handlers) DashboardPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session := auth.MustSessionFromContext(r.Context())

	kpis, err := h.svc.KPIs(r.Context())
	if err != nil {
		log.Printf("dashboard: KPIs: %v", err)
		http.Error(w, "Error loading dashboard", http.StatusInternalServerError)
		return
	}
	recents, err := h.svc.RecentSamples(r.Context(), 5)
	if err != nil {
		log.Printf("dashboard: RecentSamples: %v", err)
		http.Error(w, "Error loading dashboard", http.StatusInternalServerError)
		return
	}
	bookings, err := h.svc.TodayBookings(r.Context(), h.Now(), h.loc)
	if err != nil {
		log.Printf("dashboard: TodayBookings: %v", err)
		http.Error(w, "Error loading dashboard", http.StatusInternalServerError)
		return
	}

	var feed []ActivityEntry
	if h.activity != nil {
		entries, err := h.activity.Recent(r.Context(), 7)
		if err != nil {
			log.Printf("dashboard: activity.Recent: %v", err)
		} else {
			feed = make([]ActivityEntry, 0, len(entries))
			for _, e := range entries {
				feed = append(feed, ActivityEntry{
					ID:            e.ID,
					ActorUsername: e.ActorUsername,
					Verb:          e.Verb,
					TargetType:    e.TargetType,
					TargetID:      e.TargetID,
					TargetLabel:   e.TargetLabel,
					CreatedAt:     e.CreatedAt,
				})
			}
		}
	}

	var isAdmin bool
	if h.admin != nil {
		isAdmin, _ = h.admin.IsAdmin(r.Context(), session.UserID)
	}

	data := PageData{
		BasePageData:  BasePageData{Username: session.Username, UserID: session.UserID, IsAdmin: isAdmin},
		KPIs:          kpis,
		RecentSamples: recents,
		Activity:      feed,
		TodayBookings: bookings,
		Now:           h.Now().In(h.loc),
	}

	if err := h.renderer.Render(w, "dashboard", data); err != nil {
		log.Printf("dashboard: render: %v", err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}
