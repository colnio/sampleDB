-- Migration 009: Refined-design additions
-- Adds sample status, equipment status, wiki tags, equipment↔group access
-- and the activity_log table that powers the Dashboard & Sample-detail feeds.

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
