package dbschema

import (
	"context"
	"fmt"
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

// TestDataStatementsIncludeEquipmentSeed verifies that the Go bootstrap path
// seeds the default equipment rows just as DDL/init.sql does.
func TestDataStatementsIncludeEquipmentSeed(t *testing.T) {
	combined := strings.Join(dataStatements, "\n")
	required := []string{
		"INSERT INTO equipment",
		"'SEM'",
		"'TEM'",
		"'XRD'",
		"'AFM'",
	}
	for _, fragment := range required {
		if !strings.Contains(combined, fragment) {
			t.Fatalf("dataStatements missing equipment seed fragment: %q", fragment)
		}
	}
}

// TestDataStatementsIncludeGroupBackfill verifies that the Go bootstrap path
// populates the groups table from existing users, just as DDL/init.sql does.
func TestDataStatementsIncludeGroupBackfill(t *testing.T) {
	combined := strings.Join(dataStatements, "\n")
	required := []string{
		"INSERT INTO groups",
		"FROM users",
		`"group"`,
		"ON CONFLICT",
	}
	for _, fragment := range required {
		if !strings.Contains(combined, fragment) {
			t.Fatalf("dataStatements missing group backfill fragment: %q", fragment)
		}
	}
}

// TestSchemaBootstrapCoversInitSQLTables spot-checks that every table declared
// in DDL/init.sql is also created by schemaStatements.
func TestSchemaBootstrapCoversInitSQLTables(t *testing.T) {
	combined := strings.Join(schemaStatements, "\n")
	tables := []string{
		"users",
		"groups",
		"samples",
		"attachments",
		"articles",
		"article_attachments",
		"equipment",
		"user_equipment_permissions",
		"bookings",
		"fabrication_steps",
		"api_tokens",
		"sessions",
	}
	for _, table := range tables {
		if !strings.Contains(combined, table) {
			t.Fatalf("schemaStatements missing table: %q", table)
		}
	}
}

// TestEnsure_SchemaStatementError verifies that Ensure returns an error when a
// schema statement fails, and stops executing further statements.
func TestEnsure_SchemaStatementError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	// First schema statement succeeds, second fails.
	mock.ExpectExec(regexp.QuoteMeta(schemaStatements[0])).
		WillReturnResult(pgxmock.NewResult("EXEC", 1))
	mock.ExpectExec(regexp.QuoteMeta(schemaStatements[1])).
		WillReturnError(fmt.Errorf("schema exec error"))

	if err := Ensure(context.Background(), mock); err == nil {
		t.Fatal("expected error when schema statement fails")
	}
}

// TestEnsure_DataStatementError verifies that Ensure returns an error when a
// data statement fails after all schema statements succeed.
func TestEnsure_DataStatementError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	defer mock.Close()

	for _, stmt := range schemaStatements {
		mock.ExpectExec(regexp.QuoteMeta(stmt)).
			WillReturnResult(pgxmock.NewResult("EXEC", 1))
	}
	// First data statement fails.
	mock.ExpectExec(regexp.QuoteMeta(dataStatements[0])).
		WillReturnError(fmt.Errorf("data exec error"))

	if err := Ensure(context.Background(), mock); err == nil {
		t.Fatal("expected error when data statement fails")
	}
}

// TestSchemaBootstrapCoversLatestMigrations verifies that migrations 006 and 007
// (added after init.sql was last updated) are present in schemaStatements.
func TestSchemaBootstrapCoversLatestMigrations(t *testing.T) {
	combined := strings.Join(schemaStatements, "\n")
	required := []string{
		"equipment_text",        // migration 006
		"btree_gist",            // migration 007
		"bookings_no_overlap",   // migration 007
		"sessions",              // migration 008
		"idx_users_email_lower", // migration 012
	}
	for _, fragment := range required {
		if !strings.Contains(combined, fragment) {
			t.Fatalf("schemaStatements missing migration fragment: %q", fragment)
		}
	}
}
