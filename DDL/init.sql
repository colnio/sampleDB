-- Core account tables
CREATE TABLE IF NOT EXISTS users (
    user_id SERIAL PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    is_approved BOOLEAN DEFAULT false,
    deleted BOOLEAN DEFAULT false,
    "group" TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    admin BOOLEAN DEFAULT false
);

CREATE TABLE IF NOT EXISTS groups (
    group_id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Samples and attachments
CREATE TABLE IF NOT EXISTS samples (
    sample_id SERIAL PRIMARY KEY,
    sample_name VARCHAR(100) NOT NULL,
    sample_description TEXT,
    sample_prep TEXT,
    sample_keywords VARCHAR(255),
    sample_owner VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS attachments (
    attachment_id SERIAL PRIMARY KEY,
    source_id INT,
    attachment_address VARCHAR(255) NOT NULL,
    source_type VARCHAR(20) NOT NULL DEFAULT 'sample',
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Wiki
CREATE TABLE IF NOT EXISTS articles (
    article_id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL UNIQUE,
    content TEXT NOT NULL,
    created_by INT REFERENCES users(user_id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_modified_by INT REFERENCES users(user_id)
);

CREATE TABLE IF NOT EXISTS article_attachments (
    attachment_id SERIAL PRIMARY KEY,
    article_id INT REFERENCES articles(article_id) ON DELETE CASCADE,
    attachment_address VARCHAR(255) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    uploaded_by INT REFERENCES users(user_id)
);

-- Equipment booking
CREATE TABLE IF NOT EXISTS equipment (
    equipment_id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    location VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_equipment_permissions (
    user_id INT REFERENCES users(user_id),
    equipment_id INT REFERENCES equipment(equipment_id),
    granted_by INT REFERENCES users(user_id),
    granted_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, equipment_id)
);

CREATE TABLE IF NOT EXISTS bookings (
    booking_id SERIAL PRIMARY KEY,
    equipment_id INT REFERENCES equipment(equipment_id),
    user_id INT REFERENCES users(user_id),
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    purpose TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT valid_time_range CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_bookings_time_range
ON bookings (equipment_id, start_time, end_time);

CREATE TABLE IF NOT EXISTS fabrication_steps (
    step_id        SERIAL PRIMARY KEY,
    sample_id      INT NOT NULL REFERENCES samples(sample_id) ON DELETE CASCADE,
    step_order     INT NOT NULL DEFAULT 0,
    step_date      DATE,
    person_user_id INT REFERENCES users(user_id) ON DELETE SET NULL,
    description    TEXT,
    equipment_id   INT REFERENCES equipment(equipment_id) ON DELETE SET NULL,
    created_at     TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_tokens (
    token_id     SERIAL PRIMARY KEY,
    user_id      INT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    token_hash   VARCHAR(64) NOT NULL UNIQUE,
    name         VARCHAR(100),
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMP,
    revoked      BOOLEAN NOT NULL DEFAULT false
);

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS deleted BOOLEAN DEFAULT false;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS "group" TEXT;

ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS owner_user_id INT REFERENCES users(user_id) ON DELETE SET NULL;

ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS share_token VARCHAR(64) UNIQUE;

ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS share_visibility VARCHAR(20) NOT NULL DEFAULT 'private';

ALTER TABLE attachments
    ADD COLUMN IF NOT EXISTS source_id INT;

ALTER TABLE attachments
    ADD COLUMN IF NOT EXISTS source_type VARCHAR(20) NOT NULL DEFAULT 'sample';

DO $$ BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_schema = current_schema()
           AND table_name = 'attachments'
           AND column_name = 'sample_id'
    ) THEN
        UPDATE attachments SET source_id = sample_id WHERE source_id IS NULL;
    END IF;
END $$;

DO $$ BEGIN
    ALTER TABLE samples
        ADD CONSTRAINT chk_share_visibility
        CHECK (share_visibility IN ('private', 'public', 'authenticated'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Migration 006: add free-text equipment field to fabrication steps
ALTER TABLE fabrication_steps
    ADD COLUMN IF NOT EXISTS equipment_text TEXT;

-- Migration 007: prevent overlapping bookings via exclusion constraint
CREATE EXTENSION IF NOT EXISTS btree_gist;

DO $$ BEGIN
    ALTER TABLE bookings
        ADD CONSTRAINT bookings_no_overlap
        EXCLUDE USING gist (
            equipment_id WITH =,
            tstzrange(start_time, end_time, '[)') WITH &&
        );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Seed default equipment ONLY when the database is completely empty
-- (no users, no samples, and no equipment already present).
DO $$ BEGIN
    IF (SELECT COUNT(*) FROM users) = 0
       AND (SELECT COUNT(*) FROM samples) = 0
       AND (SELECT COUNT(*) FROM equipment) = 0
    THEN
        INSERT INTO equipment (name, description, location) VALUES
            ('SEM', 'Scanning Electron Microscope', 'Room 101'),
            ('TEM', 'Transmission Electron Microscope', 'Room 102'),
            ('XRD', 'X-Ray Diffractometer', 'Room 103'),
            ('AFM', 'Atomic Force Microscope', 'Room 104')
        ON CONFLICT (name) DO NOTHING;
    END IF;
END $$;

INSERT INTO groups (name)
SELECT DISTINCT btrim("group")
FROM users
WHERE "group" IS NOT NULL
  AND btrim("group") <> ''
ON CONFLICT (name) DO NOTHING;

-- Migration 008: persist sessions in PostgreSQL
CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    user_id    INT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    username   TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    csrf_token TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions (user_id);

-- Migration 009: Refined-design additions — sample/equipment status, wiki tags,
-- equipment↔group access, activity log.
ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'active';

DO $$ BEGIN
    ALTER TABLE samples
        ADD CONSTRAINT chk_sample_status
        CHECK (status IN ('active', 'in_progress', 'archived'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE equipment
    ADD COLUMN IF NOT EXISTS status VARCHAR(20) NOT NULL DEFAULT 'online';

DO $$ BEGIN
    ALTER TABLE equipment
        ADD CONSTRAINT chk_equipment_status
        CHECK (status IN ('online', 'offline', 'maintenance'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE articles
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS equipment_group_access (
    equipment_id INT REFERENCES equipment(equipment_id) ON DELETE CASCADE,
    group_id     INT REFERENCES groups(group_id)       ON DELETE CASCADE,
    granted_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (equipment_id, group_id)
);

CREATE TABLE IF NOT EXISTS activity_log (
    id              BIGSERIAL PRIMARY KEY,
    actor_user_id   INT REFERENCES users(user_id) ON DELETE SET NULL,
    actor_username  TEXT NOT NULL,
    verb            VARCHAR(40) NOT NULL,
    target_type     VARCHAR(40) NOT NULL,
    target_id       INT,
    target_label    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_activity_log_created_at
    ON activity_log (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_activity_log_target
    ON activity_log (target_type, target_id);

-- Migration 010: admin notification banners
CREATE TABLE IF NOT EXISTS notifications (
    id         BIGSERIAL PRIMARY KEY,
    message    TEXT NOT NULL,
    level      VARCHAR(20) NOT NULL DEFAULT 'info'
                   CHECK (level IN ('info', 'warning', 'critical')),
    author     TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Migration 011: add 'planned' to sample status values
ALTER TABLE samples DROP CONSTRAINT IF EXISTS chk_sample_status;
ALTER TABLE samples
    ADD CONSTRAINT chk_sample_status
    CHECK (status IN ('active', 'in_progress', 'archived', 'planned'));
