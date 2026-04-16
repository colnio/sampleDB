package main

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
)

// TestGetAllSamples_ContextCancellation verifies that getAllSamples propagates
// a cancelled context to the underlying DB query rather than using its own
// context.Background().  When the context is already cancelled, the pool
// returns context.Canceled and the function must surface that error.
func TestGetAllSamples_ContextCancellation(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	defer mock.Close()
	restore := setMockDB(mock)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// The mock should receive the query and return context.Canceled to simulate
	// a DB driver honouring context cancellation.
	mock.ExpectQuery(`SELECT s\.sample_id`).
		WillReturnError(context.Canceled)

	_, queryErr := getAllSamples(ctx, "", "")
	if !errors.Is(queryErr, context.Canceled) {
		t.Errorf("getAllSamples with cancelled ctx: got %v, want context.Canceled", queryErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}
