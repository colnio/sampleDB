-- Migration 005: Add CHECK constraint on share_visibility to enforce valid values
ALTER TABLE samples
    ADD CONSTRAINT chk_share_visibility
    CHECK (share_visibility IN ('private', 'public', 'authenticated'));
