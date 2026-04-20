//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/admin"
	"sampleDB/internal/app"
	"sampleDB/internal/dbschema"
)

func bcryptGen(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func TestAdmin_SeedDefaultAdminExists(t *testing.T) {
	// The integration test setup inserts a test user with admin=true.
	// Verify that at least one admin user exists.
	var count int
	if err := pool.QueryRow(bgCtx,
		"SELECT COUNT(*) FROM users WHERE admin = true AND COALESCE(deleted, false) = false").Scan(&count); err != nil {
		t.Fatalf("query admin count: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one admin user; none found")
	}
}

func TestAdmin_SeedDefaultAdmin_Idempotent(t *testing.T) {
	cfg := app.LoadConfig()
	cfg.AdminUsername = testUser.username
	cfg.AdminPassword = "does-not-matter"
	// Users table is non-empty, so SeedDefaultAdmin must be a no-op.
	seeded, err := app.SeedDefaultAdmin(bgCtx, pool, cfg)
	if err != nil {
		t.Fatalf("SeedDefaultAdmin: %v", err)
	}
	if seeded {
		t.Fatal("SeedDefaultAdmin should not seed when users already exist")
	}
}

func TestAdmin_EnsureSchemaIdempotent(t *testing.T) {
	// Running Ensure twice should not fail.
	if err := dbschema.Ensure(bgCtx, pool); err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
}

func TestAdmin_ListUsersEndpoint(t *testing.T) {
	resp := apiDo(t, http.MethodGet, "/api/v1/users", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/users status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Fatalf("missing Content-Type")
	}

	var envelope struct {
		Items []admin.APIUser `json:"items"`
		Total int             `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	found := false
	for _, u := range envelope.Items {
		if u.Username == testUser.username {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("test user %q not found in /api/v1/users", testUser.username)
	}
}

// TestAdmin_EquipmentGroupAccess seeds an equipment and a group, grants group
// access via the new handler, asserts the admin page renders the badge, then
// revokes it and asserts the badge is gone.
func TestAdmin_EquipmentGroupAccess(t *testing.T) {
	ctx := bgCtx

	// Seed a new equipment.
	eqName := "integ-eq-" + randomHex(4)
	var equipmentID int
	if err := pool.QueryRow(ctx,
		"INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id",
		eqName).Scan(&equipmentID); err != nil {
		t.Fatalf("seed equipment: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM equipment WHERE equipment_id = $1", equipmentID) })

	// Seed a new group.
	groupName := "integ-grp-" + randomHex(4)
	var groupID int
	if err := pool.QueryRow(ctx,
		"INSERT INTO groups (name) VALUES ($1) RETURNING group_id",
		groupName).Scan(&groupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM groups WHERE group_id = $1", groupID) })
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM equipment_group_access WHERE equipment_id=$1 AND group_id=$2", equipmentID, groupID)
	})

	// Log the admin user in via the web session so /admin checks pass.
	// The test user is already an admin (see setup_test.go).
	// Reset their password so we can log in normally.
	plainPassword := "integadminpass1"
	hash, err := bcryptGen(plainPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE user_id=$2", hash, testUser.id); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	cookie := login(t, testUser.username, plainPassword)

	// Grant the group access via POST /admin/equipment/{id}/groups/add.
	grantForm := url.Values{"group_id": {itoa(groupID)}}
	grantReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/admin/equipment/"+itoa(equipmentID)+"/groups/add",
		strings.NewReader(grantForm.Encode()))
	grantReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	grantReq.AddCookie(cookie)
	grantResp, err := noRedirectClient.Do(grantReq)
	if err != nil {
		t.Fatalf("POST grant: %v", err)
	}
	grantResp.Body.Close()
	if grantResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST grant status = %d", grantResp.StatusCode)
	}

	// Assert the row exists in equipment_group_access.
	var rowCount int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM equipment_group_access WHERE equipment_id=$1 AND group_id=$2",
		equipmentID, groupID).Scan(&rowCount); err != nil {
		t.Fatalf("query equipment_group_access: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("expected 1 equipment_group_access row, got %d", rowCount)
	}

	// Fetch /admin and assert both equipment and group name appear.
	adminReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin", nil)
	adminReq.AddCookie(cookie)
	adminResp, err := http.DefaultClient.Do(adminReq)
	if err != nil {
		t.Fatalf("GET /admin: %v", err)
	}
	body := mustReadBody(t, adminResp)
	if adminResp.StatusCode != http.StatusOK {
		t.Fatalf("GET /admin status = %d", adminResp.StatusCode)
	}
	if !strings.Contains(body, eqName) {
		t.Errorf("admin page missing equipment name %q", eqName)
	}
	if !strings.Contains(body, groupName) {
		t.Errorf("admin page missing group badge %q", groupName)
	}
	if !strings.Contains(body, "group-badge") {
		t.Error("admin page missing group-badge CSS class")
	}

	// Now revoke the access.
	revokeForm := url.Values{"group_id": {itoa(groupID)}}
	revokeReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/admin/equipment/"+itoa(equipmentID)+"/groups/remove",
		strings.NewReader(revokeForm.Encode()))
	revokeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	revokeReq.AddCookie(cookie)
	revokeResp, err := noRedirectClient.Do(revokeReq)
	if err != nil {
		t.Fatalf("POST revoke: %v", err)
	}
	revokeResp.Body.Close()
	if revokeResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST revoke status = %d", revokeResp.StatusCode)
	}

	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM equipment_group_access WHERE equipment_id=$1 AND group_id=$2",
		equipmentID, groupID).Scan(&rowCount); err != nil {
		t.Fatalf("query after revoke: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("expected 0 equipment_group_access rows after revoke, got %d", rowCount)
	}
}

// TestAdmin_SetEquipmentStatus seeds equipment, flips its status through the
// handler, and asserts the DB row reflects the change.
func TestAdmin_SetEquipmentStatus(t *testing.T) {
	ctx := bgCtx
	eqName := "integ-eq-status-" + randomHex(4)
	var equipmentID int
	if err := pool.QueryRow(ctx,
		"INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id",
		eqName).Scan(&equipmentID); err != nil {
		t.Fatalf("seed equipment: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, "DELETE FROM equipment WHERE equipment_id = $1", equipmentID) })

	plainPassword := "integstatuspass1"
	hash, err := bcryptGen(plainPassword)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET password_hash=$1 WHERE user_id=$2", hash, testUser.id); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	cookie := login(t, testUser.username, plainPassword)

	form := url.Values{"status": {"maintenance"}}
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/admin/equipment/"+itoa(equipmentID)+"/status",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, err := noRedirectClient.Do(req)
	if err != nil {
		t.Fatalf("POST status: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST status = %d", resp.StatusCode)
	}

	var got string
	if err := pool.QueryRow(ctx, "SELECT status FROM equipment WHERE equipment_id=$1", equipmentID).Scan(&got); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if got != "maintenance" {
		t.Errorf("equipment status = %q; want maintenance", got)
	}
}

func TestAdmin_ListEquipmentEndpoint(t *testing.T) {
	resp := apiDo(t, http.MethodGet, "/api/v1/equipment", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/equipment status = %d, want 200", resp.StatusCode)
	}

	var envelope struct {
		Items []admin.APIEquipment `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode equipment: %v", err)
	}
	_ = envelope
}
