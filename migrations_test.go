package main

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testDB returns a pool connected to DATABASE_URL (falls back to the dev default).
// Tests that call this function are skipped when no DB is reachable.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:app@localhost:5432/sampledb"
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Skipf("skipping DB test: cannot connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping DB test: ping failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// columnExists checks whether a column is present in the given table.
func columnExists(t *testing.T, pool *pgxpool.Pool, table, column string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = $1
			  AND column_name  = $2
		)`, table, column).Scan(&exists)
	if err != nil {
		t.Fatalf("columnExists query failed for %s.%s: %v", table, column, err)
	}
	return exists
}

// tableExists checks whether a table is present in the public schema.
func tableExists(t *testing.T, pool *pgxpool.Pool, table string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name   = $1
		)`, table).Scan(&exists)
	if err != nil {
		t.Fatalf("tableExists query failed for %s: %v", table, err)
	}
	return exists
}

// TestMigration001_FabricationSteps verifies the fabrication_steps table.
func TestMigration001_FabricationSteps(t *testing.T) {
	pool := testDB(t)

	if !tableExists(t, pool, "fabrication_steps") {
		t.Fatal("fabrication_steps table does not exist")
	}

	requiredColumns := []string{
		"step_id", "sample_id", "step_order",
		"step_date", "person_user_id", "description",
		"equipment_id", "equipment_text", "created_at",
	}
	for _, col := range requiredColumns {
		if !columnExists(t, pool, "fabrication_steps", col) {
			t.Errorf("fabrication_steps missing column: %s", col)
		}
	}
}

// TestMigration002_APITokens verifies the api_tokens table.
func TestMigration002_APITokens(t *testing.T) {
	pool := testDB(t)

	if !tableExists(t, pool, "api_tokens") {
		t.Fatal("api_tokens table does not exist")
	}

	requiredColumns := []string{
		"token_id", "user_id", "token_hash",
		"name", "created_at", "last_used_at", "revoked",
	}
	for _, col := range requiredColumns {
		if !columnExists(t, pool, "api_tokens", col) {
			t.Errorf("api_tokens missing column: %s", col)
		}
	}
}

// TestMigration003_SampleOwnerFK verifies the owner_user_id column on samples.
func TestMigration003_SampleOwnerFK(t *testing.T) {
	pool := testDB(t)

	if !columnExists(t, pool, "samples", "owner_user_id") {
		t.Fatal("samples.owner_user_id column does not exist")
	}
	// Legacy text field must still be present.
	if !columnExists(t, pool, "samples", "sample_owner") {
		t.Fatal("samples.sample_owner (legacy text field) must still exist")
	}
}

// TestMigration004_SampleSharing verifies share_token and share_visibility columns.
func TestMigration004_SampleSharing(t *testing.T) {
	pool := testDB(t)

	if !columnExists(t, pool, "samples", "share_token") {
		t.Fatal("samples.share_token column does not exist")
	}
	if !columnExists(t, pool, "samples", "share_visibility") {
		t.Fatal("samples.share_visibility column does not exist")
	}
}
