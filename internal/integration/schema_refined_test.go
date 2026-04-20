//go:build integration

package integration_test

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestRefinedDesignMigration asserts that migration 009 (the "refined design"
// additions) is present on the connected database: the new columns, the new
// tables, and the CHECK constraints that keep status values to their allowed
// vocabularies.
func TestRefinedDesignMigration_Columns(t *testing.T) {
	type col struct{ table, column string }
	required := []col{
		{"samples", "status"},
		{"equipment", "status"},
		{"articles", "tags"},
	}
	for _, c := range required {
		var exists bool
		err := pool.QueryRow(bgCtx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name   = $1
				  AND column_name  = $2
			)`, c.table, c.column).Scan(&exists)
		if err != nil {
			t.Fatalf("query %s.%s: %v", c.table, c.column, err)
		}
		if !exists {
			t.Errorf("%s.%s column missing", c.table, c.column)
		}
	}
}

func TestRefinedDesignMigration_Tables(t *testing.T) {
	tables := []string{"equipment_group_access", "activity_log"}
	for _, name := range tables {
		var exists bool
		err := pool.QueryRow(bgCtx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name   = $1
			)`, name).Scan(&exists)
		if err != nil {
			t.Fatalf("query table %s: %v", name, err)
		}
		if !exists {
			t.Errorf("%s table missing", name)
		}
	}
}

// TestRefinedDesignMigration_SampleStatusCheck inserts a sample with an invalid
// status and expects Postgres to reject it with a check_violation (SQLSTATE
// 23514).  Valid values must succeed and round-trip cleanly.
func TestRefinedDesignMigration_SampleStatusCheck(t *testing.T) {
	// Good insert — must succeed.
	var id int
	if err := pool.QueryRow(bgCtx, `
		INSERT INTO samples (sample_name, status)
		VALUES ('refined-status-ok-sample', 'in_progress')
		RETURNING sample_id`).Scan(&id); err != nil {
		t.Fatalf("insert valid status: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, `DELETE FROM samples WHERE sample_id = $1`, id) })

	// Bad insert — must fail with a check_violation.
	_, err := pool.Exec(bgCtx, `
		INSERT INTO samples (sample_name, status)
		VALUES ('refined-status-bad-sample', 'archivedx')`)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject invalid sample status")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("expected check_violation (23514); got %v", err)
	}
}

// TestRefinedDesignMigration_EquipmentStatusCheck mirrors the samples test but
// for equipment.status.  It uses a uniquely-named equipment row so that it
// does not clash with the seed rows inserted by the bootstrap.
func TestRefinedDesignMigration_EquipmentStatusCheck(t *testing.T) {
	// Good insert.
	var id int
	if err := pool.QueryRow(bgCtx, `
		INSERT INTO equipment (name, status)
		VALUES ('refined-eq-ok-' || now()::text, 'maintenance')
		RETURNING equipment_id`).Scan(&id); err != nil {
		t.Fatalf("insert valid equipment status: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, `DELETE FROM equipment WHERE equipment_id = $1`, id) })

	// Bad insert.
	_, err := pool.Exec(bgCtx, `
		INSERT INTO equipment (name, status)
		VALUES ('refined-eq-bad-' || now()::text, 'broken')`)
	if err == nil {
		t.Fatal("expected CHECK constraint to reject invalid equipment status")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("expected check_violation (23514); got %v", err)
	}
}

// TestRefinedDesignMigration_ArticleTagsDefault verifies that the tags column
// defaults to an empty text[] array when not supplied.
func TestRefinedDesignMigration_ArticleTagsDefault(t *testing.T) {
	var id int
	if err := pool.QueryRow(bgCtx, `
		INSERT INTO articles (title, content, created_by, last_modified_by)
		VALUES ('refined-tags-article-' || extract(epoch from now())::text, 'body', $1, $1)
		RETURNING article_id`, testUser.id).Scan(&id); err != nil {
		t.Fatalf("insert article: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, `DELETE FROM articles WHERE article_id = $1`, id) })

	var tags []string
	if err := pool.QueryRow(bgCtx,
		`SELECT tags FROM articles WHERE article_id = $1`, id).Scan(&tags); err != nil {
		t.Fatalf("select tags: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty tags default, got %v", tags)
	}

	// Round-trip with explicit tags.
	if _, err := pool.Exec(bgCtx,
		`UPDATE articles SET tags = $1 WHERE article_id = $2`,
		[]string{"Safety", "Equipment"}, id); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	var got []string
	if err := pool.QueryRow(bgCtx,
		`SELECT tags FROM articles WHERE article_id = $1`, id).Scan(&got); err != nil {
		t.Fatalf("select updated tags: %v", err)
	}
	if len(got) != 2 || got[0] != "Safety" || got[1] != "Equipment" {
		t.Fatalf("tags round-trip mismatch, got %v", got)
	}
}

// TestRefinedDesignMigration_ActivityLogInsert verifies the activity_log table
// accepts an insert using all of its columns.
func TestRefinedDesignMigration_ActivityLogInsert(t *testing.T) {
	var id int64
	if err := pool.QueryRow(bgCtx, `
		INSERT INTO activity_log (actor_user_id, actor_username, verb, target_type, target_id, target_label)
		VALUES ($1, $2, 'created', 'sample', 1, 'refined-smoke')
		RETURNING id`, testUser.id, testUser.username).Scan(&id); err != nil {
		t.Fatalf("insert activity_log row: %v", err)
	}
	t.Cleanup(func() { pool.Exec(bgCtx, `DELETE FROM activity_log WHERE id = $1`, id) })
	if id == 0 {
		t.Fatal("expected non-zero BIGSERIAL id from activity_log insert")
	}
}
