-- Migration 012: Add email to users (unique, case-insensitive)
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS email VARCHAR(254);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower
    ON users (lower(email)) WHERE email IS NOT NULL;
