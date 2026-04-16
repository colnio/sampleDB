package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestBookingExclusionConstraint_ConcurrentInserts verifies that two concurrent
// booking inserts for overlapping time on the same equipment produce exactly one
// success. This test requires a real database and will be skipped otherwise.
//
// The test:
//  1. Creates a test user and a test equipment row.
//  2. Fires two goroutines simultaneously, each trying to INSERT a booking for the
//     same equipment and overlapping time range.
//  3. Asserts that exactly one INSERT succeeds and the other fails with a
//     PostgreSQL exclusion_violation (code 23P01).
func TestBookingExclusionConstraint_ConcurrentInserts(t *testing.T) {
	pool := testDB(t)

	// Ensure btree_gist and the exclusion constraint are present.
	if _, err := pool.Exec(context.Background(), enableBtreeGistSQL); err != nil {
		t.Fatalf("enable btree_gist: %v", err)
	}
	if _, err := pool.Exec(context.Background(), addBookingExclusionConstraintSQL); err != nil {
		t.Fatalf("add exclusion constraint: %v", err)
	}

	// ---- setup: create a throwaway user and equipment row ----
	uname := fmt.Sprintf("conflict-test-user-%d", time.Now().UnixNano())
	var userID int
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (username, password_hash, is_approved) VALUES ($1, 'x', true) RETURNING user_id`,
		uname).Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM bookings WHERE user_id = $1`, userID)
		pool.Exec(context.Background(),
			`DELETE FROM users WHERE user_id = $1`, userID)
	})

	ename := fmt.Sprintf("conflict-test-eq-%d", time.Now().UnixNano())
	var eqID int
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO equipment (name) VALUES ($1) RETURNING equipment_id`,
		ename).Scan(&eqID); err != nil {
		t.Fatalf("insert test equipment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM equipment WHERE equipment_id = $1`, eqID)
	})

	// ---- concurrent inserts ----
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

	// ---- assertions ----
	successes := 0
	exclusionErrors := 0
	for _, r := range results {
		if r.err == nil {
			successes++
		} else {
			var pgErr *pgconn.PgError
			if errors.As(r.err, &pgErr) && pgErr.Code == "23P01" {
				exclusionErrors++
			} else {
				t.Errorf("unexpected error (not exclusion_violation): %v", r.err)
			}
		}
	}

	if successes != 1 {
		t.Errorf("expected exactly 1 successful insert, got %d", successes)
	}
	if exclusionErrors != 1 {
		t.Errorf("expected exactly 1 exclusion_violation, got %d", exclusionErrors)
	}
}

// enableBtreeGistSQL / addBookingExclusionConstraintSQL mirror the schema
// constants so the test can apply them to a test DB without importing the
// internal/dbschema package (avoids circular imports in package main tests).
const enableBtreeGistSQL = `CREATE EXTENSION IF NOT EXISTS btree_gist;`

const addBookingExclusionConstraintSQL = `
DO $$ BEGIN
    ALTER TABLE bookings
        ADD CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (
            equipment_id WITH =,
            tstzrange(start_time, end_time, '[)') WITH &&
        );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;`
