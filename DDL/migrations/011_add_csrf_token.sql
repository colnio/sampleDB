-- Migration 011: Add csrf_token to sessions for CSRF protection
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS csrf_token TEXT NOT NULL DEFAULT '';
