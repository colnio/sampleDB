package dbschema

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	createGroupsTable = `
CREATE TABLE IF NOT EXISTS groups (
    group_id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`
	backfillGroups = `
INSERT INTO groups (name)
SELECT DISTINCT btrim("group")
FROM users
WHERE "group" IS NOT NULL
  AND btrim("group") <> ''
ON CONFLICT (name) DO NOTHING;`
)

// Ensure ensures that the minimal schema required by the application exists.
func Ensure(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, createGroupsTable); err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, backfillGroups); err != nil {
		return err
	}

	return nil
}
