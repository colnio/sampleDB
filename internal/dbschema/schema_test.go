package dbschema

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v3"
)

func TestEnsureExecutesAllSchemaStatements(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	for _, stmt := range schemaStatements {
		mock.ExpectExec(regexp.QuoteMeta(stmt)).
			WillReturnResult(pgxmock.NewResult("EXEC", 1))
	}
	for _, stmt := range dataStatements {
		mock.ExpectExec(regexp.QuoteMeta(stmt)).
			WillReturnResult(pgxmock.NewResult("EXEC", 1))
	}

	if err := Ensure(context.Background(), mock); err != nil {
		t.Fatalf("Ensure returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestBackfillAttachmentSourceIDIsGuardedForLegacySchemas(t *testing.T) {
	required := []string{
		"information_schema.columns",
		"table_name = 'attachments'",
		"column_name = 'sample_id'",
		"UPDATE attachments SET source_id = sample_id",
	}
	for _, fragment := range required {
		if !strings.Contains(backfillAttachmentSourceID, fragment) {
			t.Fatalf("backfillAttachmentSourceID missing %q", fragment)
		}
	}
}
