// Package activity provides an append-only log of user actions across the
// application (sample create/update, wiki edits, bookings, attachment uploads,
// etc.). The log powers the Dashboard's "Recent activity" feed and the
// per-entity Activity panel on the Sample detail page.
package activity

import (
	"context"
	"database/sql"
	"time"

	"sampleDB/internal/dbiface"
)

// Verb is the controlled vocabulary describing what happened.
// Allowed: "created", "updated", "deleted", "booked", "uploaded",
// "published", "added_step", "edited_step", "granted", "revoked".
type Verb string

// TargetType identifies the entity that was acted upon.
// Allowed: "sample", "attachment", "fabrication_step", "booking",
// "article", "article_attachment", "equipment", "user".
type TargetType string

// Actor identifies who performed the action. ID is optional (0 for system
// actions); Username is required and denormalized so the feed still renders
// after a user row is deleted.
type Actor struct {
	ID       int
	Username string
}

// Entry is a single row in the activity log enriched for display.
type Entry struct {
	ID            int64
	ActorUserID   *int
	ActorUsername string
	Verb          string
	TargetType    string
	TargetID      *int
	TargetLabel   string
	CreatedAt     time.Time
}

// Logger writes rows to the activity_log table.
//
// A nil *Logger is safe: Log/Recent/ForTarget become no-ops, so tests and
// alternative entry points that don't wire activity tracking still compile.
type Logger struct {
	db dbiface.Pool
}

// NewLogger constructs a Logger backed by the given pool.
// Pass nil to get a no-op logger (safe to call Log on).
func NewLogger(db dbiface.Pool) *Logger {
	if db == nil {
		return nil
	}
	return &Logger{db: db}
}

// Log inserts one row into activity_log. Best-effort by design: the caller
// should ignore the returned error (or log it via slog) so that a logging
// failure never fails the user's request. targetID may be 0 to omit it;
// targetLabel may be empty.
func (l *Logger) Log(ctx context.Context, actor Actor, verb Verb, targetType TargetType, targetID int, targetLabel string) error {
	if l == nil || l.db == nil {
		return nil
	}
	var actorUserID any
	if actor.ID > 0 {
		actorUserID = actor.ID
	} else {
		actorUserID = nil
	}
	var targetIDArg any
	if targetID > 0 {
		targetIDArg = targetID
	} else {
		targetIDArg = nil
	}
	_, err := l.db.Exec(ctx, `
		INSERT INTO activity_log
		            (actor_user_id, actor_username, verb, target_type, target_id, target_label)
		     VALUES ($1, $2, $3, $4, $5, $6)`,
		actorUserID, actor.Username, string(verb), string(targetType), targetIDArg, targetLabel)
	return err
}

// Recent returns the N most recent entries, newest first. A nil Logger yields
// an empty slice (no error) so the Dashboard still renders in constrained
// test environments.
func (l *Logger) Recent(ctx context.Context, limit int) ([]Entry, error) {
	if l == nil || l.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := l.db.Query(ctx, `
		SELECT id, actor_user_id, actor_username, verb, target_type, target_id,
		       COALESCE(target_label, ''), created_at
		  FROM activity_log
		 ORDER BY created_at DESC, id DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// ForTarget returns the N most recent entries filtered to a single entity.
func (l *Logger) ForTarget(ctx context.Context, targetType string, targetID, limit int) ([]Entry, error) {
	if l == nil || l.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	rows, err := l.db.Query(ctx, `
		SELECT id, actor_user_id, actor_username, verb, target_type, target_id,
		       COALESCE(target_label, ''), created_at
		  FROM activity_log
		 WHERE target_type = $1 AND target_id = $2
		 ORDER BY created_at DESC, id DESC
		 LIMIT $3`, targetType, targetID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// scanRows is the minimal interface implemented by pgx.Rows that activity
// needs. It is kept here so the scanner can be reused by tests that inject a
// mock rows value.
type scanRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanEntries(rows scanRows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		var actorID sql.NullInt64
		var targetID sql.NullInt64
		if err := rows.Scan(&e.ID, &actorID, &e.ActorUsername, &e.Verb, &e.TargetType,
			&targetID, &e.TargetLabel, &e.CreatedAt); err != nil {
			return nil, err
		}
		if actorID.Valid {
			id := int(actorID.Int64)
			e.ActorUserID = &id
		}
		if targetID.Valid {
			id := int(targetID.Int64)
			e.TargetID = &id
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
