package admin_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/admin"
	"sampleDB/internal/auth"
	"sampleDB/internal/web"
)

func renderAdminPage(t *testing.T, svc admin.Service) *httptest.ResponseRecorder {
	t.Helper()
	baseDir, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatalf("abs templates dir: %v", err)
	}
	r, err := web.NewRenderer(baseDir, nil, time.UTC)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	h := admin.NewHandlers(svc, r, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req = req.WithContext(auth.ContextWithSession(req.Context(), auth.Session{UserID: 1, Username: "admin"}))
	rr := httptest.NewRecorder()
	h.AdminPage(rr, req)
	return rr
}

// TestAdmin_EquipmentStatusDotAndGroups verifies that the admin page renders
// a per-equipment status dot, a status select for editing, and group badges
// derived from admin.Equipment.Groups.
func TestAdmin_EquipmentStatusDotAndGroups(t *testing.T) {
	svc := &mockService{
		equipment: []admin.Equipment{
			{ID: 1, Name: "SEM", Status: "online", Groups: []string{"Barbaros", "Ahmet"}},
			{ID: 2, Name: "AFM", Status: "maintenance", Groups: []string{}},
		},
		groups: []admin.Group{
			{ID: 10, Name: "Barbaros"},
			{ID: 11, Name: "Ahmet"},
			{ID: 12, Name: "Visitors"},
		},
		users: []admin.UserAccess{},
	}
	rr := renderAdminPage(t, svc)
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()

	wants := []string{
		"equipment-status-dot--online",
		"equipment-status-dot--maintenance",
		"SEM",
		"AFM",
		// group badges
		"group-badge",
		"Barbaros",
		"Ahmet",
		// the status select + add-group dropdown form actions
		`action="/admin/equipment/1/status"`,
		`action="/admin/equipment/1/groups/add"`,
		`action="/admin/equipment/1/groups/remove"`,
		`action="/admin/equipment/2/status"`,
		// "No groups" for the AFM row (no groups assigned)
		"No groups",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("admin page render missing %q", w)
		}
	}
}

// TestAdmin_UserManagementColumns verifies that the User Management panel
// renders approval status chips, admin badge, and an equipment count.
func TestAdmin_UserManagementColumns(t *testing.T) {
	svc := &mockService{
		equipment: []admin.Equipment{
			{ID: 1, Name: "SEM", Status: "online", Groups: []string{}},
		},
		groups: []admin.Group{{ID: 10, Name: "PhDs"}},
		users: []admin.UserAccess{
			{UserID: 5, Username: "alice", Approved: true, Admin: true, GroupName: "PhDs",
				EquipmentAccess: []admin.Equipment{{ID: 1, Name: "SEM"}}},
			{UserID: 6, Username: "bob", Approved: false, Admin: false, GroupName: ""},
		},
	}
	rr := renderAdminPage(t, svc)
	if rr.Code != http.StatusOK {
		t.Fatalf("render status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()

	wants := []string{
		"alice",
		"bob",
		// Approved chip for alice
		"status-chip--active",
		// Pending chip for bob
		"status-chip--pending",
		// Admin badge for alice
		"status-chip--progress",
		// Equipment count element
		"equipment-count-value",
		"user-card__equipment-count",
	}
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("admin users panel render missing %q", w)
		}
	}
}
