-- Migration 003: Add owner_user_id FK to samples (keep legacy sample_owner text field)
ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS owner_user_id INT REFERENCES users(user_id) ON DELETE SET NULL;
