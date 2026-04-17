//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"sampleDB/internal/admin"
	"sampleDB/internal/app"
	"sampleDB/internal/dbschema"
)

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
