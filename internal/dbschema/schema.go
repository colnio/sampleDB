package dbschema

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

var schemaStatements = []string{
	createUsersTable,
	addDeletedColumn,
	addGroupColumn,
	createSamplesTable,
	createAttachmentsTable,
	createArticlesTable,
	createArticleAttachmentsTable,
	createEquipmentTable,
	createUserEquipmentPermissionsTable,
	createBookingsTable,
	createBookingsIndex,
	createGroupsTable,
}

var dataStatements = []string{
	backfillGroups,
	seedEquipment,
	backfillDeleted,
}

const createUsersTable = `
CREATE TABLE IF NOT EXISTS users (
    user_id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_approved BOOLEAN DEFAULT false,
    deleted BOOLEAN DEFAULT false,
    "group" TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    admin BOOLEAN DEFAULT false
);`

const addDeletedColumn = `
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS deleted BOOLEAN DEFAULT false;`

const addGroupColumn = `
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS "group" TEXT;`

const createSamplesTable = `
CREATE TABLE IF NOT EXISTS samples (
    sample_id SERIAL PRIMARY KEY,
    sample_name VARCHAR(100) NOT NULL,
    sample_description TEXT,
    sample_prep TEXT,
    sample_keywords VARCHAR(255),
    sample_owner VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`

const createAttachmentsTable = `
CREATE TABLE IF NOT EXISTS attachments (
    attachment_id SERIAL PRIMARY KEY,
    sample_id INT REFERENCES samples(sample_id) ON DELETE CASCADE,
    attachment_address VARCHAR(255) NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`

const createArticlesTable = `
CREATE TABLE IF NOT EXISTS articles (
    article_id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL UNIQUE,
    content TEXT NOT NULL,
    created_by INT REFERENCES users(user_id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_modified_by INT REFERENCES users(user_id)
);`

const createArticleAttachmentsTable = `
CREATE TABLE IF NOT EXISTS article_attachments (
    attachment_id SERIAL PRIMARY KEY,
    article_id INT REFERENCES articles(article_id) ON DELETE CASCADE,
    attachment_address VARCHAR(255) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    uploaded_by INT REFERENCES users(user_id)
);`

const createEquipmentTable = `
CREATE TABLE IF NOT EXISTS equipment (
    equipment_id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    location VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`

const createUserEquipmentPermissionsTable = `
CREATE TABLE IF NOT EXISTS user_equipment_permissions (
    user_id INT REFERENCES users(user_id),
    equipment_id INT REFERENCES equipment(equipment_id),
    granted_by INT REFERENCES users(user_id),
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, equipment_id)
);`

const createBookingsTable = `
CREATE TABLE IF NOT EXISTS bookings (
    booking_id SERIAL PRIMARY KEY,
    equipment_id INT REFERENCES equipment(equipment_id),
    user_id INT REFERENCES users(user_id),
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    purpose TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT valid_time_range CHECK (end_time > start_time)
);`

const createBookingsIndex = `
CREATE INDEX IF NOT EXISTS idx_bookings_time_range
ON bookings (equipment_id, start_time, end_time);`

const createGroupsTable = `
CREATE TABLE IF NOT EXISTS groups (
    group_id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);`

const backfillGroups = `
INSERT INTO groups (name)
SELECT DISTINCT btrim("group")
FROM users
WHERE "group" IS NOT NULL
  AND btrim("group") <> ''
ON CONFLICT (name) DO NOTHING;`

const seedEquipment = `
INSERT INTO equipment (name, description, location)
VALUES
    ('AFM', 'Atomic Force Microscope', 'E3A L7'),
    ('XeCl', 'XeCl MAC LCVD system', 'Laser Lab S12'),
    ('ICP', 'MAC growth system', 'Laser Lab S12'),
    ('Probe Station', 'Probe Station', 'Meeting room S13')
ON CONFLICT (name) DO NOTHING;`

const backfillDeleted = `
UPDATE users
SET deleted = false
WHERE deleted IS NULL;`

// Ensure ensures that all schema elements required by the application exist.
func Ensure(ctx context.Context, pool *pgxpool.Pool) error {
	for _, stmt := range schemaStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	for _, stmt := range dataStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}

	return nil
}
