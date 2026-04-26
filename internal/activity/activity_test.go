package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/activity"
)

func TestLogger_Nil_IsNoOp(t *testing.T) {
	var l *activity.Logger // nil
	// Must not panic and must return nil error.
	if err := l.Log(context.Background(), activity.Actor{ID: 1, Username: "a"}, "created", "sample", 1, "x"); err != nil {
		t.Fatalf("nil Log: %v", err)
	}
	got, err := l.Recent(context.Background(), 5)
	if err != nil || got != nil {
		t.Fatalf("nil Recent = (%v, %v); want (nil, nil)", got, err)
	}
	got, err = l.ForTarget(context.Background(), "sample", 1, 5)
	if err != nil || got != nil {
		t.Fatalf("nil ForTarget = (%v, %v); want (nil, nil)", got, err)
	}
	got, err = l.ForSample(context.Background(), 1, 5)
	if err != nil || got != nil {
		t.Fatalf("nil ForSample = (%v, %v); want (nil, nil)", got, err)
	}
}

func TestLogger_NewLogger_NilPool_ReturnsNil(t *testing.T) {
	if l := activity.NewLogger(nil); l != nil {
		t.Fatalf("NewLogger(nil) = %v, want nil", l)
	}
}

func TestLogger_Log_HappyPath(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	mock.ExpectExec(`INSERT INTO activity_log`).
		WithArgs(7, "alice", "created", "sample", 42, "my-sample").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := l.Log(context.Background(),
		activity.Actor{ID: 7, Username: "alice"},
		"created", "sample", 42, "my-sample"); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLogger_Log_ZeroActorAndTarget_ArePersistedAsNil(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	mock.ExpectExec(`INSERT INTO activity_log`).
		WithArgs(nil, "sys", "deleted", "article", nil, "").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := l.Log(context.Background(),
		activity.Actor{ID: 0, Username: "sys"},
		"deleted", "article", 0, ""); err != nil {
		t.Fatalf("Log: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLogger_Log_DBError_Propagates(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	boom := errors.New("db down")
	mock.ExpectExec(`INSERT INTO activity_log`).
		WithArgs(1, "u", "created", "sample", 1, "").
		WillReturnError(boom)

	err = l.Log(context.Background(),
		activity.Actor{ID: 1, Username: "u"},
		"created", "sample", 1, "")
	if !errors.Is(err, boom) {
		t.Fatalf("Log error = %v; want %v", err, boom)
	}
}

func TestLogger_Recent_ReturnsEntries(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	now := time.Now().UTC().Truncate(time.Second)
	userID := int64(4)
	tID := int64(42)

	mock.ExpectQuery(`SELECT al\.id, al\.actor_user_id`).
		WithArgs(5).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "actor_user_id", "actor_username", "verb", "target_type", "target_id", "target_label", "created_at",
		}).
			AddRow(int64(2), userID, "bob", "updated", "sample", tID, "widget", now).
			AddRow(int64(1), nil, "sys", "deleted", "article", nil, "", now.Add(-time.Minute)))

	entries, err := l.Recent(context.Background(), 5)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(entries))
	}
	if entries[0].ActorUserID == nil || *entries[0].ActorUserID != 4 {
		t.Errorf("entries[0].ActorUserID = %v; want 4", entries[0].ActorUserID)
	}
	if entries[0].TargetID == nil || *entries[0].TargetID != 42 {
		t.Errorf("entries[0].TargetID = %v; want 42", entries[0].TargetID)
	}
	if entries[1].ActorUserID != nil {
		t.Errorf("entries[1].ActorUserID = %v; want nil", entries[1].ActorUserID)
	}
	if entries[1].TargetID != nil {
		t.Errorf("entries[1].TargetID = %v; want nil", entries[1].TargetID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLogger_Recent_DefaultLimit(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	// Limit<=0 should be replaced by 10.
	mock.ExpectQuery(`SELECT al\.id, al\.actor_user_id`).
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "actor_user_id", "actor_username", "verb", "target_type", "target_id", "target_label", "created_at",
		}))

	if _, err := l.Recent(context.Background(), 0); err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLogger_ForTarget_FiltersAndReturns(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	now := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`WHERE target_type = \$1 AND target_id = \$2`).
		WithArgs("sample", 99, 3).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "actor_user_id", "actor_username", "verb", "target_type", "target_id", "target_label", "created_at",
		}).AddRow(int64(10), int64(2), "x", "uploaded", "sample", int64(99), "file.png", now))

	entries, err := l.ForTarget(context.Background(), "sample", 99, 3)
	if err != nil {
		t.Fatalf("ForTarget: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries; want 1", len(entries))
	}
	if entries[0].Verb != "uploaded" {
		t.Errorf("Verb = %q; want uploaded", entries[0].Verb)
	}
	if entries[0].TargetLabel != "file.png" {
		t.Errorf("TargetLabel = %q; want file.png", entries[0].TargetLabel)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestLogger_ForTarget_QueryError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	boom := errors.New("timeout")
	mock.ExpectQuery(`WHERE target_type = \$1 AND target_id = \$2`).
		WithArgs("sample", 1, 10).
		WillReturnError(boom)

	_, err = l.ForTarget(context.Background(), "sample", 1, 10)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v; want %v", err, boom)
	}
}

func TestLogger_ForSample_IncludesChildTargets(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	l := activity.NewLogger(mock)
	now := time.Now().UTC().Truncate(time.Second)

	mock.ExpectQuery(`fabrication_steps WHERE sample_id = \$1`).
		WithArgs(42, 6).
		WillReturnRows(pgxmock.NewRows([]string{
			"id", "actor_user_id", "actor_username", "verb", "target_type", "target_id", "target_label", "created_at",
		}).
			AddRow(int64(12), int64(7), "alice", "edited_step", "fabrication_step", int64(9), "Etch", now).
			AddRow(int64(11), int64(7), "alice", "uploaded", "attachment", int64(20), "image.png", now.Add(-time.Minute)))

	entries, err := l.ForSample(context.Background(), 42, 6)
	if err != nil {
		t.Fatalf("ForSample: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(entries))
	}
	if entries[0].TargetType != "fabrication_step" || entries[0].Verb != "edited_step" {
		t.Errorf("entries[0] = (%s, %s); want (fabrication_step, edited_step)", entries[0].TargetType, entries[0].Verb)
	}
	if entries[1].TargetType != "attachment" || entries[1].TargetLabel != "image.png" {
		t.Errorf("entries[1] = (%s, %q); want (attachment, image.png)", entries[1].TargetType, entries[1].TargetLabel)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
