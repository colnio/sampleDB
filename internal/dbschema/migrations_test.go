package dbschema

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
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

func TestMigration003_SampleOwnerFK(t *testing.T) {
	pool := testDB(t)

	if !columnExists(t, pool, "samples", "owner_user_id") {
		t.Fatal("samples.owner_user_id column does not exist")
	}
	if !columnExists(t, pool, "samples", "sample_owner") {
		t.Fatal("samples.sample_owner (legacy text field) must still exist")
	}
}

func TestMigration004_SampleSharing(t *testing.T) {
	pool := testDB(t)

	if !columnExists(t, pool, "samples", "share_token") {
		t.Fatal("samples.share_token column does not exist")
	}
	if !columnExists(t, pool, "samples", "share_visibility") {
		t.Fatal("samples.share_visibility column does not exist")
	}
}

// TestBookingExclusionConstraint_ConcurrentInserts verifies that two concurrent
// booking inserts for overlapping time on the same equipment produce exactly one
// success. This test requires a real database and will be skipped otherwise.
func TestBookingExclusionConstraint_ConcurrentInserts(t *testing.T) {
	pool := testDB(t)

	if _, err := pool.Exec(context.Background(), enableBtreeGistSQL); err != nil {
		t.Fatalf("enable btree_gist: %v", err)
	}
	if _, err := pool.Exec(context.Background(), addBookingExclusionConstraintSQL); err != nil {
		t.Fatalf("add exclusion constraint: %v", err)
	}

	uname := fmt.Sprintf("conflict-test-user-%d", time.Now().UnixNano())
	var userID int
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash, is_approved) VALUES ($1, 'x', true) RETURNING user_id`,
		uname).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	ename := fmt.Sprintf("conflict-test-eq-%d", time.Now().UnixNano())
	var eqID int
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id`,
		ename).Scan(&eqID); err != nil {
		t.Fatalf("insert test equipment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM bookings WHERE equipment_id = $1`, eqID)
		pool.Exec(context.Background(),
			`DELETE FROM equipment WHERE equipment_id = $1`, eqID)
		pool.Exec(context.Background(),
			`DELETE FROM users WHERE user_id = $1`, userID)
	})

	start := time.Now().Add(time.Hour)
	end := start.Add(time.Hour)

	type result struct {
		err error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup

	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, results[idx].err = pool.Exec(context.Background(),
				`INSERT INTO bookings (equipment_id, user_id, start_time, end_time, purpose)
				 VALUES ($1, $2, $3, $4, 'test')`,
				eqID, userID, start, end)
		}(i)
	}
	wg.Wait()

	// PostgreSQL may raise exclusion_violation (23P01) or deadlock (40P01).
	successes := 0
	constraintErrors := 0
	for _, r := range results {
		if r.err == nil {
			successes++
		} else {
			var pgErr *pgconn.PgError
			if errors.As(r.err, &pgErr) && (pgErr.Code == "23P01" || pgErr.Code == "40P01") {
				constraintErrors++
			} else {
				t.Errorf("unexpected error (expected exclusion_violation or deadlock): %v", r.err)
			}
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 successful insert, got %d", successes)
	}
	if constraintErrors != 1 {
		t.Errorf("expected exactly 1 constraint error (exclusion_violation or deadlock), got %d", constraintErrors)
	}
}

const enableBtreeGistSQL = `CREATE EXTENSION IF NOT EXISTS btree_gist;`

const addBookingExclusionConstraintSQL = `
DO $$ BEGIN
    ALTER TABLE bookings
        ADD CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (
            equipment_id WITH =,
            tstzrange(start_time, end_time, '[)') WITH &&
        );
EXCEPTION WHEN duplicate_object OR duplicate_table THEN NULL;
END $$;`

func TestMigration012_UserEmail(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()

	for _, stmt := range []string{addUserEmailColumn, createUserEmailUniqueIndex} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("apply migration 012: %v", err)
		}
	}
	if !columnExists(t, pool, "users", "email") {
		t.Fatal("users.email column does not exist")
	}

	var indexExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_users_email_lower')`,
	).Scan(&indexExists); err != nil {
		t.Fatalf("index lookup: %v", err)
	}
	if !indexExists {
		t.Fatal("idx_users_email_lower index does not exist")
	}
}
