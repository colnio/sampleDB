-- Migration 004: Add share_token and share_visibility to samples
ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS share_token VARCHAR(64) UNIQUE;

ALTER TABLE samples
    ADD COLUMN IF NOT EXISTS share_visibility VARCHAR(20) NOT NULL DEFAULT 'private';
