//go:build integration

package integration_test

import (
	"context"
	"testing"

	"sampleDB/internal/activity"
)

// TestActivityLogger_Insert_And_Query exercises the Logger against a real
// Postgres. It inserts two rows (a "created" then an "updated" for the same
// sample target) and then reads them back via Recent and ForTarget.
func TestActivityLogger_Insert_And_Query(t *testing.T) {
	ctx := context.Background()
	logger := activity.NewLogger(pool)

	// Insert a sample so target_id points at a real row (not strictly required —
	// activity_log.target_id has no FK — but mirrors production usage).
	var sampleID int
	if err := pool.QueryRow(ctx, `
		INSERT INTO samples (sample_name, owner_user_id, sample_owner)
		VALUES ('activity-integ-sample', $1, $2)
		RETURNING sample_id`, testUser.id, testUser.username).Scan(&sampleID); err != nil {
		t.Fatalf("insert sample: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM samples WHERE sample_id = $1`, sampleID) })
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM activity_log WHERE target_type = 'sample' AND target_id = $1`, sampleID) })

	actor := activity.Actor{ID: testUser.id, Username: testUser.username}
	if err := logger.Log(ctx, actor, "created", "sample", sampleID, "activity-integ-sample"); err != nil {
		t.Fatalf("Log(created): %v", err)
	}
	if err := logger.Log(ctx, actor, "updated", "sample", sampleID, "activity-integ-sample"); err != nil {
		t.Fatalf("Log(updated): %v", err)
	}

	// ForTarget — must see both rows, newest first.
	entries, err := logger.ForTarget(ctx, "sample", sampleID, 10)
	if err != nil {
		t.Fatalf("ForTarget: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ForTarget len = %d; want 2", len(entries))
	}
	verbs := []string{entries[0].Verb, entries[1].Verb}
	// Newest first; both inserts share the same created_at so order is by id DESC.
	if verbs[0] != "updated" || verbs[1] != "created" {
		t.Errorf("verb order = %v; want [updated created]", verbs)
	}
	for _, e := range entries {
		if e.ActorUsername != testUser.username {
			t.Errorf("entry.ActorUsername = %q; want %q", e.ActorUsername, testUser.username)
		}
		if e.TargetID == nil || *e.TargetID != sampleID {
			t.Errorf("entry.TargetID = %v; want %d", e.TargetID, sampleID)
		}
		if e.TargetLabel != "activity-integ-sample" {
			t.Errorf("entry.TargetLabel = %q; want activity-integ-sample", e.TargetLabel)
		}
	}

	// Recent — our rows must appear at the head (most-recent).
	recent, err := logger.Recent(ctx, 50)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	var seen int
	for _, e := range recent {
		if e.TargetType == "sample" && e.TargetID != nil && *e.TargetID == sampleID {
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("Recent saw %d of our rows; want 2 (out of %d returned)", seen, len(recent))
	}
}

// TestActivityLogger_SampleCreate_WritesRow exercises the full HTTP pipeline:
// a POST /api/v1/samples request should produce a single "created" row in
// activity_log for that sample.
func TestActivityLogger_SampleCreate_WritesRow(t *testing.T) {
	ctx := context.Background()

	resp := apiDo(t, "POST", "/api/v1/samples",
		jsonBody(`{"name":"activity-integ-http-create","description":"","keywords":""}`))
	if resp.StatusCode != 201 {
		t.Fatalf("create sample: status=%d body=%s", resp.StatusCode, mustReadBody(t, resp))
	}
	resp.Body.Close()

	var sampleID int
	if err := pool.QueryRow(ctx,
		`SELECT sample_id FROM samples WHERE sample_name = 'activity-integ-http-create'`,
	).Scan(&sampleID); err != nil {
		t.Fatalf("lookup sample: %v", err)
	}
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM samples WHERE sample_id = $1`, sampleID) })
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM activity_log WHERE target_type = 'sample' AND target_id = $1`, sampleID) })

	// Now PUT to rename it — must add a second ("updated") activity row.
	resp = apiDo(t, "PUT", "/api/v1/samples/"+itoa(sampleID),
		jsonBody(`{"name":"activity-integ-http-updated","description":"","keywords":""}`))
	if resp.StatusCode != 200 {
		t.Fatalf("update sample: status=%d body=%s", resp.StatusCode, mustReadBody(t, resp))
	}
	resp.Body.Close()

	var verbs []string
	rows, err := pool.Query(ctx,
		`SELECT verb FROM activity_log WHERE target_type = 'sample' AND target_id = $1 ORDER BY id ASC`,
		sampleID)
	if err != nil {
		t.Fatalf("query activity_log: %v", err)
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		verbs = append(verbs, v)
	}
	rows.Close()

	if len(verbs) != 2 || verbs[0] != "created" || verbs[1] != "updated" {
		t.Errorf("verbs = %v; want [created updated]", verbs)
	}
}

func itoa(i int) string {
	// strconv.Itoa, but avoid an extra import in this file.
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
